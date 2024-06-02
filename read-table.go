//go:build ignore

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	t, err := readRoutingTable(context.Background())
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	fmt.Printf("t: %v\n", t)
}

var netstatRE = regexp.MustCompile(`(?P<dest>[0-9.]+|default)\s+(?P<gw>[0-9a-f:]+)\s+(?P<flags>\w+)\s+(?P<if>\w+)\s+(?P<exp>\d+)*`)

type routeEntry struct {
	dst   string
	gw    string
	flags string
	iface string
	exp   time.Duration
}

func readRoutingTable(ctx context.Context) (table []routeEntry, err error) {
	out, err := exec.CommandContext(ctx, "netstat", "-rn").Output()
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		l := sc.Text()
		if len(l) == 0 || strings.HasPrefix(l, "Internet") || strings.HasPrefix(l, "Destination") {
			continue
		}
		matches := netstatRE.FindStringSubmatch(l)
		//fmt.Printf("matches: %v: %v\n", len(matches), matches)
		var entry routeEntry
		switch len(matches) {
		case 6:
			if len(matches[5]) > 0 {
				exp, _ := time.ParseDuration(matches[5] + "s")
				entry.exp = exp
				fmt.Printf("EXP %v - %v\n", matches[5], exp)
			}
			fmt.Printf("EXP %v - %v\n", matches[5], entry.exp)
			fallthrough
		case 5:
			entry.dst = matches[1]
			entry.gw = matches[2]
			entry.flags = matches[3]
			entry.iface = matches[4]
		default:
			continue
		}
		table = append(table, entry)
	}
	return table, sc.Err()
}
