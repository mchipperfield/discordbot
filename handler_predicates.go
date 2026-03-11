package main

import (
	"regexp"
	"strings"
)

// Package-level compiled regexes — compiled once at startup, not per message.
var (
	kitRegex      = regexp.MustCompile(`\bkit\b`)
	fullSendRegex = regexp.MustCompile(`\bfull send\b`)
	listenRegex   = regexp.MustCompile(`\blisten\b`)
)

func isQuoteCommand(content string) bool {
	return content == "!quote"
}

func isHungry(content string) bool {
	return strings.Contains(strings.ToLower(content), "hungry")
}

func isTired(content string) bool {
	return strings.Contains(strings.ToLower(content), "tired")
}

func isKit(content string) bool {
	return kitRegex.MatchString(strings.ToLower(content))
}

func isFullSend(content string) bool {
	return fullSendRegex.MatchString(strings.ToLower(content))
}

func isListen(content string) bool {
	return listenRegex.MatchString(strings.ToLower(content))
}
