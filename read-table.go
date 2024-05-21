//go:build ignore

package main

import (
	"context"
	"fmt"
)

func main() {
	t, err := readARPTable(context.Background())
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	fmt.Printf("t: %v\n", t)
}
