package job

import (
	"os"
	"strings"
	"testing"
)

func banLineCount(t *testing.T, email string) int {
	t.Helper()
	body, err := os.ReadFile(readIpLimitLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read 3xipl.log: %v", err)
	}
	return strings.Count(string(body), "[LIMIT_IP] Email = "+email)
}
