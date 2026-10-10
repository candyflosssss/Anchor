// Command desktop-contract emits the Wails boundary's TypeScript from Go DTOs.
package main

import (
	"fmt"
	"github.com/candyflosssss/Anchor/internal/desktop"
)

func main() { fmt.Print(desktop.TypeScript()) }
