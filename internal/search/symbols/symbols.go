package symbols

// Symbol represents a code symbol (function, type, variable, etc.)
type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`      // function, type, variable, etc.
	Path      string `json:"path"`      // file path
	Line      int    `json:"line"`      // 1-indexed line number
	Language  string `json:"language"`  // detected language
	Pattern   string `json:"pattern"`   // search pattern (ctags output)
	Scope     string `json:"scope"`     // parent scope (e.g., class name)
	Signature string `json:"signature"` // function signature if available
}

// FindSymbolResult is the result of a symbol search
type FindSymbolResult struct {
	Symbols []Symbol `json:"symbols"`
}

// ListDefsResult is the result of listing definitions in a file
type ListDefsResult struct {
	Path    string   `json:"path"`
	Symbols []Symbol `json:"symbols"`
}
