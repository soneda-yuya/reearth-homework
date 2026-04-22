package errs

// Redact masks a sensitive string so it is safe to include in logs or errors.
//
// Strings of 8 characters or fewer collapse to "[REDACTED]" entirely so that
// short tokens cannot be reconstructed from even the 2-char hint. Longer
// strings keep only the first two and last two characters, e.g.
// "sk-ant-abcdef1234" -> "sk...34".
func Redact(s string) string {
	if len(s) <= 8 {
		return "[REDACTED]"
	}
	return s[:2] + "..." + s[len(s)-2:]
}
