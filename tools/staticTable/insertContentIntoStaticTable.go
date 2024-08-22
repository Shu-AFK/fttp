package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	var path = flag.String("content", "", "The content of the file to insert")
	flag.Parse()

	if *path == "" {
		panic("The file path is required")
	}

	f, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		splitLine := strings.Split(scanner.Text(), ";")

		for _, element := range splitLine {
			element = strings.Trim(element, ";")
			element = strings.TrimSpace(element)
		}

		fmt.Printf("IndexAddressSpace_[%v] = NewHeaderField(\"%v\", \"%v\", false)\n", splitLine[0], splitLine[1], splitLine[2])
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
