package version

import (
	"fmt"
	"testing"
)

func TestGetVersion(t *testing.T) {
	fmt.Println("Version Information:")
	fmt.Println(GetVersion())
}
