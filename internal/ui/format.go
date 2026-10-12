package ui

import (
	"fmt"
	"strings"
)

// Placeholder texts shown instead of a value.
const (
	notDetected = "Nezjištěno"
	loadingText = "Načítání..."
)

// textOrUnknown returns the text, or "Nezjištěno" when it is blank.
func textOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return notDetected
	}
	return value
}

// countOrUnknown formats a count where 0 means "could not be detected".
func countOrUnknown(value uint32) string {
	if value == 0 {
		return notDetected
	}
	return fmt.Sprintf("%d", value)
}

// formatFrequency formats MHz as GHz with a Czech decimal comma: "3,60 GHz".
func formatFrequency(megahertz uint32) string {
	if megahertz == 0 {
		return notDetected
	}
	return czechDecimal(float64(megahertz)/1000) + " GHz"
}

// formatCapacity formats bytes as GB with a Czech decimal comma: "15,86 GB".
// Zero means "could not be detected", unless zeroIsKnown says that zero is a
// real value (a graphics card can have no dedicated memory).
func formatCapacity(bytes uint64, zeroIsKnown bool) string {
	if bytes == 0 && !zeroIsKnown {
		return notDetected
	}
	return czechDecimal(float64(bytes)/(1024*1024*1024)) + " GB"
}

// czechDecimal formats a number with two decimals and a decimal comma.
func czechDecimal(value float64) string {
	return strings.ReplaceAll(fmt.Sprintf("%.2f", value), ".", ",")
}
