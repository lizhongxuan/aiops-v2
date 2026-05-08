package terminalpolicy

import (
	"net/url"
	"path/filepath"
	"strings"
	"unicode"
)

// IsReadOnlyCommand classifies terminal invocations that are safe to run in
// chat/inspect contexts without approval. It intentionally considers args for
// tools like curl where the executable alone is not enough to decide safety.
func IsReadOnlyCommand(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if wrappedCommand, wrappedArgs, ok := unwrapReadOnlyShell(base, args); ok {
		return IsReadOnlyCommand(wrappedCommand, wrappedArgs)
	}
	if base == "curl" {
		return isReadOnlyCurlArgs(args)
	}
	if base == "ifconfig" {
		return isReadOnlyIfconfigArgs(args)
	}
	if base == "sed" {
		return isReadOnlySedArgs(args)
	}
	if base == "sysctl" {
		return isReadOnlySysctlArgs(args)
	}
	return IsReadOnlyCommandName(command)
}

func IsReadOnlyCommandName(command string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	switch base {
	case "cat", "date", "df", "du", "echo", "find", "free", "grep", "head", "hostname", "id", "ls", "nproc", "printf", "ps", "pwd", "rg", "stat", "sw_vers", "tail", "top", "uname", "uptime", "vm_stat", "wc", "which", "whoami":
		return true
	default:
		return false
	}
}

func isReadOnlyIfconfigArgs(args []string) bool {
	if len(args) == 0 {
		return true
	}
	seenInterface := false
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" || strings.ContainsAny(trimmed, "\x00\n\r`$<>;|&=") {
			return false
		}
		if strings.HasPrefix(trimmed, "-") {
			switch trimmed {
			case "-a", "-l", "-m", "-v":
				continue
			default:
				return false
			}
		}
		if seenInterface || !isSafeInterfaceName(trimmed) {
			return false
		}
		seenInterface = true
	}
	return true
}

func isSafeInterfaceName(value string) bool {
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == ':') {
			return false
		}
	}
	return value != ""
}

func isReadOnlySysctlArgs(args []string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			return false
		}
		if trimmed == "-w" || strings.HasPrefix(trimmed, "-w") || strings.Contains(trimmed, "=") {
			return false
		}
		if strings.HasPrefix(trimmed, "-") {
			switch trimmed {
			case "-a", "-A", "-n":
				continue
			default:
				return false
			}
		}
		if strings.ContainsAny(trimmed, "\x00\n\r`$<>;|&") {
			return false
		}
	}
	return true
}

func isReadOnlySedArgs(args []string) bool {
	if len(args) < 2 {
		return false
	}
	scriptIndex := 0
	if strings.TrimSpace(args[0]) == "-n" {
		scriptIndex = 1
	}
	if scriptIndex >= len(args)-1 {
		return false
	}
	if !isSafeSedPrintScript(args[scriptIndex]) {
		return false
	}
	for _, arg := range args[scriptIndex+1:] {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.ContainsAny(trimmed, "\x00\n\r`$<>;|&") {
			return false
		}
	}
	return true
}

func isSafeSedPrintScript(script string) bool {
	script = strings.TrimSpace(script)
	if !strings.HasSuffix(script, "p") || strings.Contains(script, "w") || strings.Contains(script, "W") {
		return false
	}
	body := strings.TrimSuffix(script, "p")
	if body == "" {
		return false
	}
	for _, r := range body {
		if !(unicode.IsDigit(r) || r == ',' || r == '$') {
			return false
		}
	}
	return true
}

func unwrapReadOnlyShell(base string, args []string) (string, []string, bool) {
	switch base {
	case "bash", "sh", "zsh":
	default:
		return "", nil, false
	}
	if len(args) != 2 {
		return "", nil, false
	}
	switch strings.TrimSpace(args[0]) {
	case "-c", "-lc":
	default:
		return "", nil, false
	}
	command, commandArgs, ok := SplitCommandLine(args[1])
	if !ok {
		return "", nil, false
	}
	return command, commandArgs, true
}

// SplitCommandLine parses the model-friendly single-string command form into
// exec.Command-compatible command + args without invoking a shell. It accepts
// simple single/double quoted tokens, but rejects shell control syntax.
func SplitCommandLine(line string) (string, []string, bool) {
	tokens, ok := splitCommandLineFields(line)
	if !ok || len(tokens) == 0 {
		return "", nil, false
	}
	if hasUnsafeCommandLineTokens(tokens) {
		return "", nil, false
	}
	return tokens[0], append([]string(nil), tokens[1:]...), true
}

func splitCommandLineFields(line string) ([]string, bool) {
	var tokens []string
	var current strings.Builder
	var quote rune

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for _, r := range strings.TrimSpace(line) {
		if r == '\x00' || r == '\n' || r == '\r' {
			return nil, false
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, false
	}
	flush()
	return tokens, true
}

func hasUnsafeCommandLineTokens(tokens []string) bool {
	for _, token := range tokens {
		if token == "" {
			return true
		}
		switch token {
		case "&&", "||", "|", ";", "<", ">", ">>", "2>", "&":
			return true
		}
		if strings.ContainsAny(token, "\x00\n\r`$<>;|") || strings.Contains(token, "&&") || strings.Contains(token, "||") {
			return true
		}
		if strings.Contains(token, "&") && !tokenIsSafeURLArgument(token) {
			return true
		}
	}
	return false
}

func tokenIsSafeURLArgument(token string) bool {
	if isHTTPURLString(token) {
		return true
	}
	if strings.HasPrefix(token, "--url=") {
		return isHTTPURLString(strings.TrimPrefix(token, "--url="))
	}
	return false
}

func isReadOnlyCurlArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	sawURL := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			return false
		}
		if isHTTPURLString(arg) {
			sawURL = true
			continue
		}
		if strings.HasPrefix(arg, "--url=") {
			if !isHTTPURLString(strings.TrimPrefix(arg, "--url=")) {
				return false
			}
			sawURL = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if isDeniedCurlFlag(arg) {
				return false
			}
			if isAllowedCurlBoolFlag(arg) {
				continue
			}
			if flagNeedsValue, valueIsURL := allowedCurlValueFlag(arg); flagNeedsValue {
				i++
				if i >= len(args) {
					return false
				}
				value := strings.TrimSpace(args[i])
				if value == "" {
					return false
				}
				if valueIsURL {
					if !isHTTPURLString(value) {
						return false
					}
					sawURL = true
				}
				continue
			}
			if allowedCurlInlineValueFlag(arg) {
				continue
			}
			return false
		}
		return false
	}
	return sawURL
}

func isAllowedCurlBoolFlag(flag string) bool {
	switch flag {
	case "-s", "-S", "-sS", "-Ss", "-L", "-I", "-f", "--silent", "--show-error", "--location", "--head", "--get", "--compressed", "--fail", "--fail-with-body":
		return true
	default:
		return false
	}
}

func allowedCurlValueFlag(flag string) (bool, bool) {
	switch flag {
	case "-m", "--max-time", "--connect-timeout", "--retry", "--retry-delay", "-H", "--header", "-A", "--user-agent":
		return true, false
	case "--url":
		return true, true
	default:
		return false, false
	}
}

func allowedCurlInlineValueFlag(flag string) bool {
	for _, prefix := range []string{
		"--max-time=",
		"--connect-timeout=",
		"--retry=",
		"--retry-delay=",
		"--header=",
		"--user-agent=",
	} {
		if strings.HasPrefix(flag, prefix) && strings.TrimSpace(strings.TrimPrefix(flag, prefix)) != "" {
			return true
		}
	}
	return false
}

func isDeniedCurlFlag(flag string) bool {
	deniedExact := map[string]bool{
		"-X": true, "--request": true,
		"-d": true, "--data": true, "--data-raw": true, "--data-binary": true, "--data-urlencode": true, "--json": true,
		"-F": true, "--form": true, "--form-string": true,
		"-T": true, "--upload-file": true,
		"-o": true, "-O": true, "--output": true, "--remote-name": true,
		"-K": true, "--config": true,
		"-u": true, "--user": true, "--oauth2-bearer": true,
		"-b": true, "-c": true, "--cookie": true, "--cookie-jar": true,
		"-x": true, "--proxy": true,
	}
	if deniedExact[flag] {
		return true
	}
	for _, prefix := range []string{
		"-X", "--request=", "--data", "--json=", "--form", "--upload-file=", "--output=", "--remote-name-all",
		"--config=", "--user=", "--oauth2-bearer=", "--cookie", "--proxy=",
	} {
		if strings.HasPrefix(flag, prefix) {
			return true
		}
	}
	return false
}

func isHTTPURLString(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Host == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}
