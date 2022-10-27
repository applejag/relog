package config

type Config struct {
	Patterns []Pattern
}

type Pattern struct {
	LeadingTimestamp *PatternLeadingTimestamp
	JSON             *PatternJSON
	LogFmt           *PatternLogFmt
}

type PatternLeadingTimestamp struct {
	Layouts []string
	Trim    bool
}

type PatternJSON struct {
}

type PatternLogFmt struct {
}
