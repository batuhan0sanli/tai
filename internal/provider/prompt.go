package provider

// systemPrompt constrains every backend to emit only a raw, executable command.
// It is the system message for API providers and is prepended to the request
// for CLI providers. Keep the "output must be ready to run as-is" contract:
// cmd/root.go feeds the returned string straight into `bash -c` and clipboards,
// and SanitizeCommand is the only thing standing between a model-side prompt
// injection and arbitrary shell execution under -y.
const systemPrompt = "You are a terminal command generator. For the request below, output ONLY the raw, executable terminal command. Do not include greetings, explanations, markdown formatting (```), or backticks. The output must be a command ready to run as-is."

// cliPrompt combines the system prompt and the user request into a single
// string, for CLI backends that take one positional prompt argument.
func cliPrompt(request string) string {
	return systemPrompt + "\n\nRequest: " + request
}
