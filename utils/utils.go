package utils

import (
	"fmt"
	"strings"
)

func Strike(s string) string {
	if len(s) == 0 {
		return s
	}

	return fmt.Sprintf("\u0336%s\u0336", strings.Join(strings.Split(s, ""), "\u0336"))
}
