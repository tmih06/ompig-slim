package pigversion

// HarnessVersion is the personal application release, independent of the PiG SDK
// pin. Release builds override it with -ldflags -X; source builds keep this pin.
var HarnessVersion = "0.1.9"
