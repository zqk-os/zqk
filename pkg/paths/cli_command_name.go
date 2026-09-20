package paths

// CLICommandNameDefault is the default executable name for the CLI.
// It can be overridden at runtime during initialization to support white-labeling.
const CLICommandNameDefault = "zqk"

// CLICommandName is the executable/command name used in help text, examples, and suggestions.
// It is set during CLI initialization from config (brand.executable_name) or the actual binary name.
var CLICommandName = CLICommandNameDefault
