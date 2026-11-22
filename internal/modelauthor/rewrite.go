package modelauthor

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

const trailerPrefix = "Co-Authored-By: "

// commitFlags returns what to append after `git commit` for the identity, given
// the commit's own arguments. A flag the caller already supplied is left alone.
func commitFlags(identity string, args []string) string {
	trailer := trailerPrefix + identity
	var author, trailed bool
	for _, arg := range args {
		arg = unquote(arg)
		author = author || arg == "--author" || strings.HasPrefix(arg, "--author=")
		trailed = trailed || strings.Contains(arg, trailer)
	}
	var flags string
	if !author {
		flags += " --author=" + shellQuote(identity)
	}
	if !trailed {
		flags += " --trailer=" + shellQuote(trailer)
	}
	return flags
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// rewriteCommits adds the author and trailer flags for identity after every
// `git commit` that sits at a command position. It reads just enough shell to
// leave text alone that only looks like a command: quoted strings, comments,
// heredoc bodies, and nested $(...) are tracked, so a commit message that
// mentions "git commit" is not rewritten. It returns command unchanged when
// there is nothing to add.
func rewriteCommits(command, identity string) string {
	s := &scanner{src: command, identity: identity}
	s.seq(0, 0)
	if len(s.inserts) == 0 {
		return command
	}
	// A commit nested in $(...) is recorded before the one that contains it.
	slices.SortFunc(s.inserts, func(a, b insertion) int { return cmp.Compare(a.at, b.at) })
	var out strings.Builder
	last := 0
	for _, ins := range s.inserts {
		out.WriteString(command[last:ins.at])
		out.WriteString(ins.text)
		last = ins.at
	}
	out.WriteString(command[last:])
	return out.String()
}

type insertion struct {
	at   int
	text string
}

type scanner struct {
	src      string
	identity string
	inserts  []insertion
}

type heredoc struct {
	delim string
	dash  bool
}

const wordEnd = " \t\r\n;&|()<>"

// keepsCommandPosition reports whether the next word is still a command after
// word: shell keywords and prefixes that introduce one.
func keepsCommandPosition(word string) bool {
	switch word {
	case "if", "then", "else", "elif", "do", "while", "until", "!", "{", "time":
		return true
	}
	return false
}

// takesValue reports whether a git global option consumes the following word.
func takesValue(option string) bool {
	switch option {
	case "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--super-prefix", "--exec-path":
		return true
	}
	return false
}

// seq scans a command list and returns the offset of closer, or the end of the
// input when it is absent.
func (s *scanner) seq(i int, closer byte) int {
	atCommand := true
	var pending []heredoc
	for i < len(s.src) {
		c := s.src[i]
		switch {
		case closer != 0 && c == closer:
			return i
		case c == '\n':
			i = s.skipHeredocs(i+1, pending)
			pending = nil
			atCommand = true
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '\\':
			i = min(i+2, len(s.src))
		case c == '#' && (i == 0 || strings.IndexByte(" \t\n;&|(", s.src[i-1]) >= 0):
			for i < len(s.src) && s.src[i] != '\n' {
				i++
			}
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			i++
			atCommand = true
		case c == '<' && strings.HasPrefix(s.src[i:], "<<") && !strings.HasPrefix(s.src[i:], "<<<"):
			h, end := s.heredocStart(i + 2)
			if h.delim != "" {
				pending = append(pending, h)
			}
			i = end
		case c == '<' || c == '>':
			i++
		default:
			end := s.word(i)
			word := s.src[i:end]
			i = end
			if atCommand && !keepsCommandPosition(word) && !isAssignment(word) {
				if word == "git" {
					i = s.git(i)
				}
				atCommand = false
			}
		}
	}
	return i
}

var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// isAssignment reports whether word is NAME=value. The substring test keeps the
// regexp off the many command words that are not assignments.
func isAssignment(word string) bool {
	return strings.Contains(word, "=") && assignment.MatchString(word)
}

// word returns the end of the shell word starting at i.
func (s *scanner) word(i int) int {
	for i < len(s.src) {
		c := s.src[i]
		switch {
		case c == '\'':
			i = s.skipSingle(i)
		case c == '"':
			i = s.skipDouble(i)
		case c == '\\':
			i = min(i+2, len(s.src))
		case c == '`':
			i = s.skipBacktick(i)
		case c == '$' && strings.HasPrefix(s.src[i:], "$("):
			i = min(s.seq(i+2, ')')+1, len(s.src))
		case c == '$' && strings.HasPrefix(s.src[i:], "${"):
			end := strings.IndexByte(s.src[i:], '}')
			if end < 0 {
				return len(s.src)
			}
			i += end + 1
		case strings.IndexByte(wordEnd, c) >= 0:
			return i
		default:
			i++
		}
	}
	return i
}

func (s *scanner) skipSingle(i int) int {
	end := strings.IndexByte(s.src[i+1:], '\'')
	if end < 0 {
		return len(s.src)
	}
	return i + end + 2
}

func (s *scanner) skipDouble(i int) int {
	i++
	for i < len(s.src) {
		switch {
		case s.src[i] == '\\':
			i += 2
		case s.src[i] == '"':
			return i + 1
		case s.src[i] == '`':
			i = s.skipBacktick(i)
		case strings.HasPrefix(s.src[i:], "$("):
			i = s.seq(i+2, ')') + 1
		default:
			i++
		}
	}
	return len(s.src)
}

func (s *scanner) skipBacktick(i int) int {
	i++
	for i < len(s.src) {
		switch s.src[i] {
		case '\\':
			i += 2
		case '`':
			return i + 1
		default:
			i++
		}
	}
	return len(s.src)
}

// heredocStart parses the delimiter after "<<" and returns where the operator ends.
func (s *scanner) heredocStart(i int) (heredoc, int) {
	var h heredoc
	if i < len(s.src) && s.src[i] == '-' {
		h.dash = true
		i++
	}
	for i < len(s.src) && (s.src[i] == ' ' || s.src[i] == '\t') {
		i++
	}
	end := s.word(i)
	h.delim = strings.NewReplacer("'", "", `"`, "", "\\", "").Replace(s.src[i:end])
	return h, end
}

// skipHeredocs consumes the bodies of the heredocs opened on the previous line.
func (s *scanner) skipHeredocs(i int, pending []heredoc) int {
	for _, h := range pending {
		for i < len(s.src) {
			lineEnd := strings.IndexByte(s.src[i:], '\n')
			line := s.src[i:]
			if lineEnd < 0 {
				i = len(s.src)
			} else {
				line = s.src[i : i+lineEnd]
				i += lineEnd + 1
			}
			if h.dash {
				line = strings.TrimLeft(line, "\t")
			}
			if strings.TrimSuffix(line, "\r") == h.delim {
				break
			}
		}
	}
	return i
}

// nextWord returns the start of the next word on the same command, or false at a
// separator or the end of the input.
func (s *scanner) nextWord(i int) (int, bool) {
	for i < len(s.src) {
		switch {
		case s.src[i] == ' ' || s.src[i] == '\t' || s.src[i] == '\r':
			i++
		case strings.HasPrefix(s.src[i:], "\\\n"):
			i += 2
		default:
			return i, strings.IndexByte("\n;&|()<>", s.src[i]) < 0
		}
	}
	return i, false
}

type span struct{ start, end int }

// words returns the remaining words of the current command and the offset where
// they stop.
func (s *scanner) words(i int) ([]span, int) {
	var out []span
	for {
		start, ok := s.nextWord(i)
		if !ok {
			return out, start
		}
		i = s.word(start)
		out = append(out, span{start, i})
	}
}

// git reads the arguments after a "git" word once. Reaching a commit
// subcommand, it records the flags to insert after that word. It returns where
// the arguments end so the caller does not scan them again.
func (s *scanner) git(i int) int {
	words, stop := s.words(i)
	skipValue := false
	for n, w := range words {
		text := unquote(s.src[w.start:w.end])
		switch {
		case skipValue:
			skipValue = false
		case takesValue(text):
			skipValue = true
		case strings.HasPrefix(text, "-"):
		case text == "commit":
			args := make([]string, 0, len(words)-n-1)
			for _, a := range words[n+1:] {
				args = append(args, s.src[a.start:a.end])
			}
			if flags := commitFlags(s.identity, args); flags != "" {
				s.inserts = append(s.inserts, insertion{w.end, flags})
			}
			return stop
		default:
			return stop
		}
	}
	return stop
}
