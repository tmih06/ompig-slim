#!/usr/bin/env python3
"""Regress PTY packet fragmentation without a terminal, credentials or sleeps."""
import importlib.util
import pathlib
import unittest
from unittest.mock import Mock, patch


SPEC = importlib.util.spec_from_file_location(
    "measure_model_menu", pathlib.Path(__file__).with_name("measure-model-menu.py"))
MENU = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MENU)
TITLE = b"All available models"
BEGIN = b"\x1b[?2026h"
END = b"\x1b[?2026l"


class ModelMenuWaitTests(unittest.TestCase):
    """Model OS read boundaries independently from synchronized renderer frames."""

    def setUp(self):
        """Use a running child; individual tests can simulate an early exit."""
        self.process = Mock()
        self.process.poll.return_value = None

    def observe(self, chunks, complete_frame=True):
        """Feed one-second-spaced packets on a fake clock; never wait on real IO."""
        packets = iter(chunks)
        clock = [0.0]

        def read_chunk(_fd, _size):
            """Advance time only when another OS packet is consumed."""
            clock[0] += 1
            return next(packets)

        with (patch.object(MENU.os, "read", side_effect=read_chunk),
              patch.object(MENU.os, "write"),
              patch.object(MENU.select, "select", return_value=([7], [], [])),
              patch.object(MENU.time, "monotonic", side_effect=lambda: clock[0])):
            return MENU.wait_for(7, self.process, TITLE, complete_frame=complete_frame)

    def test_complete_frame_waits_past_title_without_inflating_latency(self):
        """An older frame end and a title-only packet must not satisfy readiness."""
        chunks = [END + BEGIN + TITLE, b"\r\nopenai", END[:4], END[4:]]
        output, visible_at = self.observe(chunks)
        self.assertEqual(output, b"".join(chunks))
        self.assertIn(b"openai", output)
        self.assertEqual(visible_at, 1.0)

    def test_every_frame_split_preserves_content(self):
        """Accept every possible read split, including inside the title/end CSI."""
        frame = BEGIN + TITLE + b"\r\nopenai\r\n" + END
        for boundary in range(1, len(frame)):
            with self.subTest(boundary=boundary):
                output, _ = self.observe([frame[:boundary], frame[boundary:]])
                self.assertEqual(output, frame)

    def test_marker_time_is_when_the_complete_marker_arrives(self):
        """Do not timestamp a partial title, or the later frame completion."""
        chunks = [BEGIN + TITLE[:5], TITLE[5:], b"openai" + END]
        output, visible_at = self.observe(chunks)
        self.assertEqual(output, b"".join(chunks))
        self.assertEqual(visible_at, 2.0)

    def test_literal_wait_does_not_require_a_frame(self):
        """Startup, switching and alternate-screen exit keep literal semantics."""
        output, visible_at = self.observe([TITLE, b"not needed"], complete_frame=False)
        self.assertEqual(output, TITLE)
        self.assertEqual(visible_at, 1.0)
        self.process.poll.assert_not_called()

    def test_exit_after_title_is_not_a_complete_frame(self):
        """A dying child must fail instead of passing on its partial header."""
        self.process.poll.return_value = 1
        with self.assertRaisesRegex(AssertionError, "complete frame.*exit=1"):
            self.observe([BEGIN + TITLE])

    def test_eof_after_title_is_not_a_complete_frame(self):
        """A closed PTY without a frame end must fail even before child reaping."""
        with self.assertRaisesRegex(AssertionError, "complete frame"):
            self.observe([BEGIN + TITLE, b""])


if __name__ == "__main__":
    unittest.main()
