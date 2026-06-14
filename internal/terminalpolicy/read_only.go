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
	if base == "docker" {
		return isReadOnlyDockerArgs(args)
	}
	if base == "ifconfig" {
		return isReadOnlyIfconfigArgs(args)
	}
	if base == "sed" {
		return isReadOnlySedArgs(args)
	}
	if base == "ss" {
		return isAllowedSSArgs(args)
	}
	if base == "sysctl" {
		return isReadOnlySysctlArgs(args)
	}
	return IsReadOnlyCommandName(command)
}

// IsAllowedReadOnlyTerminal is the break-glass terminal allowlist. It is
// intentionally narrower than IsReadOnlyCommand: even read-only terminal usage
// must be operationally scoped before it can run without approval.
func IsAllowedReadOnlyTerminal(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if wrappedCommand, wrappedArgs, ok := unwrapReadOnlyShell(base, args); ok {
		return IsAllowedReadOnlyTerminal(wrappedCommand, wrappedArgs)
	}
	switch base {
	case "kubectl":
		return isAllowedReadOnlyKubectlArgs(args)
	case "curl":
		return isReadOnlyCurlArgs(args)
	case "docker":
		return isReadOnlyDockerArgs(args)
	case "redis-cli":
		return isAllowedReadOnlyRedisCLIArgs(args)
	default:
		return false
	}
}

// IsAllowedHostInspectionTerminal classifies bounded host resource/status
// inspection commands that are safe to execute without an ActionToken. This is
// narrower than IsReadOnlyCommand and avoids broad file reads such as cat.
func IsAllowedHostInspectionTerminal(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if wrappedCommand, wrappedArgs, ok := unwrapReadOnlyShell(base, args); ok {
		return IsAllowedHostInspectionTerminal(wrappedCommand, wrappedArgs)
	}
	switch base {
	case "uptime", "vm_stat", "lscpu", "nproc", "hostname", "whoami", "id", "uname", "sw_vers":
		return allSafeTerminalTokens(args)
	case "df", "du":
		return isAllowedHostInspectionWithFlags(args, map[string]bool{
			"-h": true, "-H": true, "-k": true, "-m": true, "-g": true, "-T": true,
		})
	case "free":
		return isAllowedHostInspectionWithFlags(args, map[string]bool{
			"-h": true, "-m": true, "-g": true, "-b": true, "-k": true,
		})
	case "top":
		return isAllowedTopArgs(args)
	case "ps":
		return isAllowedHostInspectionWithFlags(args, map[string]bool{
			"-a": true, "-u": true, "-x": true, "-e": true, "-f": true, "-l": true,
		})
	case "lsof":
		return isAllowedLsofArgs(args)
	case "ss":
		return isAllowedSSArgs(args)
	case "sysctl":
		return isReadOnlySysctlArgs(args) && allSafeTerminalTokens(args)
	case "ifconfig":
		return isReadOnlyIfconfigArgs(args)
	default:
		return false
	}
}

func isAllowedHostInspectionWithFlags(args []string, allowedFlags map[string]bool) bool {
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || !isSafeTerminalToken(arg) {
			return false
		}
		if strings.HasPrefix(arg, "-") && !allowedFlags[arg] {
			return false
		}
	}
	return true
}

func isAllowedLsofArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	seenNetworkSelector := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || !isSafeTerminalToken(arg) {
			return false
		}
		switch {
		case arg == "-n" || arg == "-P":
			continue
		case arg == "-i":
			i++
			if i >= len(args) || !isSafeLsofInternetSelector(args[i]) {
				return false
			}
			seenNetworkSelector = true
		case strings.HasPrefix(arg, "-i") && len(arg) > len("-i"):
			if !isSafeLsofInternetSelector(strings.TrimPrefix(arg, "-i")) {
				return false
			}
			seenNetworkSelector = true
		case strings.HasPrefix(arg, "-sTCP:"):
			if !isSafeTerminalToken(strings.TrimPrefix(arg, "-sTCP:")) {
				return false
			}
		default:
			return false
		}
	}
	return seenNetworkSelector
}

func isSafeLsofInternetSelector(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.HasPrefix(value, "-") && strings.Contains(value, ":") && isSafeTerminalToken(value)
}

func isAllowedTopArgs(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || !isSafeTerminalToken(arg) {
			return false
		}
		switch arg {
		case "-l", "-n", "-s", "-o":
			i++
			if i >= len(args) || !isSafeTerminalToken(args[i]) {
				return false
			}
		case "-b":
			continue
		case "-stats":
			i++
			if i >= len(args) || !isSafeTerminalToken(args[i]) {
				return false
			}
		default:
			if strings.HasPrefix(arg, "-") {
				return false
			}
		}
	}
	return true
}

func isAllowedSSArgs(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || strings.ContainsAny(arg, "\x00\n\r`$<>;|&") {
			return false
		}
		if strings.HasPrefix(arg, "-") {
			if !isSafeSSFlag(arg) {
				return false
			}
			continue
		}
		switch arg {
		case "sport", "dport":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) != "=" {
				return false
			}
			i++
			if i >= len(args) || !isSafeSSPort(args[i]) {
				return false
			}
		case "state":
			i++
			if i >= len(args) || !isSafeSSState(args[i]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func isSafeSSFlag(flag string) bool {
	flag = strings.TrimSpace(flag)
	if len(flag) < 2 || !strings.HasPrefix(flag, "-") || strings.HasPrefix(flag, "--") {
		return false
	}
	for _, r := range flag[1:] {
		if !strings.ContainsRune("tulnpaH46", r) {
			return false
		}
	}
	return true
}

func isSafeSSPort(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), ":")
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isSafeSSState(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "listening", "established", "syn-sent", "syn-recv", "time-wait", "close-wait":
		return true
	default:
		return false
	}
}

// TerminalRiskLevel returns the minimum approval risk for a terminal command.
// Mutating or non-allowlisted terminal commands are always high risk because
// terminal access is break-glass diagnostic fallback, not a default action path.
func TerminalRiskLevel(command string, args []string) string {
	if IsAllowedReadOnlyTerminal(command, args) || IsAllowedHostInspectionTerminal(command, args) {
		return "low"
	}
	return "high"
}

func RequiresHighRiskApproval(command string, args []string) bool {
	return TerminalRiskLevel(command, args) == "high"
}

func IsReadOnlyCommandName(command string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	switch base {
	case "cat", "date", "df", "du", "echo", "find", "free", "grep", "head", "hostname", "id", "ls", "lsof", "lscpu", "nproc", "printf", "ps", "pwd", "rg", "stat", "sw_vers", "tail", "top", "uname", "uptime", "vm_stat", "wc", "which", "whoami":
		return true
	default:
		return false
	}
}

func isAllowedReadOnlyKubectlArgs(args []string) bool {
	if len(args) < 2 {
		return false
	}
	verb := strings.TrimSpace(args[0])
	switch verb {
	case "get", "describe":
		return kubectlArgsAreSafe(args[1:])
	case "logs":
		return kubectlLogsArgsAreSafe(args[1:])
	default:
		return false
	}
}

func kubectlLogsArgsAreSafe(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || strings.ContainsAny(arg, "\x00\n\r`$<>;|&") {
			return false
		}
		switch arg {
		case "-f", "--follow":
			return false
		case "-n", "--namespace", "-c", "--container", "--context", "--since", "--tail", "--limit-bytes":
			i++
			if i >= len(args) || !isSafeTerminalToken(args[i]) {
				return false
			}
		default:
			if strings.HasPrefix(arg, "--tail=") || strings.HasPrefix(arg, "--since=") ||
				strings.HasPrefix(arg, "--limit-bytes=") || strings.HasPrefix(arg, "--container=") ||
				strings.HasPrefix(arg, "--namespace=") || strings.HasPrefix(arg, "--context=") {
				_, value, _ := strings.Cut(arg, "=")
				if !isSafeTerminalToken(value) {
					return false
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return false
			}
			if !isSafeTerminalToken(arg) {
				return false
			}
		}
	}
	return true
}

func kubectlArgsAreSafe(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || strings.ContainsAny(arg, "\x00\n\r`$<>;|&") {
			return false
		}
		switch arg {
		case "-n", "--namespace", "-l", "--selector", "-o", "--output", "--context", "--field-selector":
			i++
			if i >= len(args) || !isSafeTerminalToken(args[i]) {
				return false
			}
		case "-A", "--all-namespaces", "--show-labels", "--watch=false":
			continue
		default:
			if strings.HasPrefix(arg, "--namespace=") || strings.HasPrefix(arg, "--selector=") ||
				strings.HasPrefix(arg, "--output=") || strings.HasPrefix(arg, "--context=") ||
				strings.HasPrefix(arg, "--field-selector=") {
				_, value, _ := strings.Cut(arg, "=")
				if !isSafeTerminalToken(value) {
					return false
				}
				continue
			}
			if arg == "-w" || arg == "--watch" || strings.HasPrefix(arg, "--watch=") {
				return false
			}
			if strings.HasPrefix(arg, "-") {
				return false
			}
			if !isSafeTerminalToken(arg) {
				return false
			}
		}
	}
	return true
}

func isAllowedReadOnlyRedisCLIArgs(args []string) bool {
	commandIndex := -1
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || strings.ContainsAny(arg, "\x00\n\r`$<>;|&") {
			return false
		}
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "-h", "-p", "-n", "-u", "-a", "--user", "--pass", "--tls":
				if arg != "--tls" {
					i++
					if i >= len(args) || !isSafeTerminalToken(args[i]) {
						return false
					}
				}
				continue
			default:
				return false
			}
		}
		commandIndex = i
		break
	}
	if commandIndex < 0 {
		return false
	}
	cmd := strings.ToUpper(strings.TrimSpace(args[commandIndex]))
	switch cmd {
	case "INFO":
		return len(args[commandIndex+1:]) <= 1 && allSafeTerminalTokens(args[commandIndex+1:])
	case "MEMORY":
		return len(args[commandIndex+1:]) == 1 && strings.EqualFold(args[commandIndex+1], "STATS")
	default:
		return false
	}
}

func isReadOnlyDockerArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	subcommand := strings.TrimSpace(args[0])
	switch subcommand {
	case "ps":
		return dockerPSArgsAreSafe(args[1:])
	case "container":
		if len(args) < 2 {
			return false
		}
		switch strings.TrimSpace(args[1]) {
		case "ls", "ps":
			return dockerPSArgsAreSafe(args[2:])
		default:
			return false
		}
	case "inspect":
		return len(args) > 1 && dockerInspectArgsAreSafe(args[1:])
	case "version", "info":
		return allSafeTerminalTokens(args[1:])
	default:
		return false
	}
}

func dockerPSArgsAreSafe(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			return false
		}
		switch arg {
		case "-a", "--all", "-q", "--quiet", "--no-trunc", "--size", "-l", "--latest":
			continue
		case "-f", "--filter":
			i++
			if i >= len(args) || !isSafeDockerFilter(args[i]) {
				return false
			}
		case "--format":
			i++
			if i >= len(args) || !isSafeDockerFormat(args[i]) {
				return false
			}
		case "-n", "--last":
			i++
			if i >= len(args) || !isSafeDockerNumber(args[i]) {
				return false
			}
		default:
			switch {
			case strings.HasPrefix(arg, "--filter="):
				if !isSafeDockerFilter(strings.TrimPrefix(arg, "--filter=")) {
					return false
				}
			case strings.HasPrefix(arg, "--format="):
				if !isSafeDockerFormat(strings.TrimPrefix(arg, "--format=")) {
					return false
				}
			case strings.HasPrefix(arg, "--last="):
				if !isSafeDockerNumber(strings.TrimPrefix(arg, "--last=")) {
					return false
				}
			default:
				return false
			}
		}
	}
	return true
}

func dockerInspectArgsAreSafe(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			return false
		}
		switch arg {
		case "--format", "-f":
			i++
			if i >= len(args) || !isSafeDockerFormat(args[i]) {
				return false
			}
		default:
			if strings.HasPrefix(arg, "--format=") {
				if !isSafeDockerFormat(strings.TrimPrefix(arg, "--format=")) {
					return false
				}
				continue
			}
			if strings.HasPrefix(arg, "-") || !isSafeDockerIdentifier(arg) {
				return false
			}
		}
	}
	return true
}

func isSafeDockerFilter(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00\n\r`$<>;|&")
}

func isSafeDockerFormat(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00\n\r`$<>;|&")
}

func isSafeDockerIdentifier(value string) bool {
	return isSafeTerminalToken(value)
}

func isSafeDockerNumber(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func allSafeTerminalTokens(values []string) bool {
	for _, value := range values {
		if !isSafeTerminalToken(value) {
			return false
		}
	}
	return true
}

func isSafeTerminalToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\n\r`$<>;|&") {
		return false
	}
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-./:=,@%", r)) {
			return false
		}
	}
	return true
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
			if isAllowedCurlNullOutputFlag(arg) {
				if arg == "-o" || arg == "--output" {
					i++
					if i >= len(args) || strings.TrimSpace(args[i]) != "/dev/null" {
						return false
					}
				}
				continue
			}
			if isAllowedCurlWriteOutFlag(arg) {
				if arg == "-w" || arg == "--write-out" {
					i++
					if i >= len(args) || !isSafeCurlWriteOut(args[i]) {
						return false
					}
				}
				continue
			}
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
	if isAllowedCurlShortBoolCombo(flag) {
		return true
	}
	switch flag {
	case "-s", "-S", "-sS", "-Ss", "-L", "-I", "-f", "--silent", "--show-error", "--location", "--head", "--get", "--compressed", "--fail", "--fail-with-body":
		return true
	default:
		return false
	}
}

func isAllowedCurlShortBoolCombo(flag string) bool {
	flag = strings.TrimSpace(flag)
	if len(flag) < 3 || !strings.HasPrefix(flag, "-") || strings.HasPrefix(flag, "--") {
		return false
	}
	for _, r := range flag[1:] {
		if !strings.ContainsRune("fsSLI", r) {
			return false
		}
	}
	return true
}

func isAllowedCurlNullOutputFlag(flag string) bool {
	switch flag {
	case "-o", "--output":
		return true
	default:
		return strings.TrimSpace(flag) == "--output=/dev/null"
	}
}

func isAllowedCurlWriteOutFlag(flag string) bool {
	switch flag {
	case "-w", "--write-out":
		return true
	default:
		if strings.HasPrefix(flag, "--write-out=") {
			return isSafeCurlWriteOut(strings.TrimPrefix(flag, "--write-out="))
		}
		return false
	}
}

func isSafeCurlWriteOut(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00\n\r`$<>;|&")
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
		"-O": true, "--remote-name": true,
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
