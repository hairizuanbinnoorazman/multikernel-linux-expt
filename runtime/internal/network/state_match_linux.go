//go:build linux

package network

import "strings"

// endpointStateMatches compares complete iproute2 tokens, not substrings. Link
// UP means the administrative flag in <...>, not LOWER_UP or state text. Routes
// and links must have one unambiguous observation; address listings may include
// other address families. This is sampled state, not an incarnation attestation.
func endpointStateMatches(output []byte, want []string) bool {
	if len(want) == 0 {
		return true
	}
	text := strings.TrimSpace(string(output))
	if len(want) == 1 && want[0] == "1" {
		return text == "1"
	}
	lines := strings.Split(text, "\n")
	if len(want) > 1 && len(lines) != 1 {
		return false
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		matches := true
		for _, expected := range want {
			found := false
			if expected == "UP" {
				// ip -o link has the flags immediately after index and interface name.
				if len(fields) > 2 && strings.HasPrefix(fields[2], "<") && strings.HasSuffix(fields[2], ">") {
					for _, flag := range strings.Split(fields[2][1:len(fields[2])-1], ",") {
						if flag == "UP" {
							found = true
						}
					}
				}
			} else {
				tokens := strings.Fields(expected)
				for i := 0; i+len(tokens) <= len(fields); i++ {
					if strings.Join(fields[i:i+len(tokens)], " ") == expected {
						found = true
						break
					}
				}
			}
			matches = matches && found
		}
		if matches {
			return true
		}
	}
	return false
}
