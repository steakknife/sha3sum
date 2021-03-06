package main

// sha3sum [options] [files...]
//
//
// With no FILE, or when FILE is -, read standard input.
//
// -a 224
// -a 256 (default)
// -a 384
// -a 512
//
// -c check
//
// -s silent
//

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"github.com/droundy/goopt"
	"github.com/steakknife/securecompare"
	"hash"
	"io"
	"os"
	"regexp"
	"strconv"
)

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func dieerr(err error) {
	die(fmt.Sprint(err))
}

const BUF_SIZE = 256 * 1024

func hashFile(filename string, algorithm int) (result string, err error) {
	var f *os.File
	if filename == "-" {
		f = os.Stdin
	} else {
		f, err = os.Open(filename)
		if err != nil {
			dieerr(err)
		}
	}
	defer f.Close()

	var h hash.Hash
	switch algorithm {
	case 224:
		h = New224()
	case 256:
		h = New256()
	case 384:
		h = New384()
	case 512:
		h = New512()
	}

	buf := make([]byte, BUF_SIZE)
	for {
		n, err2 := f.Read(buf)
		if err2 != nil {
			if err2 != io.EOF {
				err = err2
			}
			break
		} else if n > 0 {
			h.Write(buf[:n])
		}
	}

	if err != nil {
		dieerr(err)
	}

	result = ""
	sum := h.Sum(nil)
	for _, b := range sum {
		result += fmt.Sprintf("%02x", b)
	}
	return
}

var tagRegexp = regexp.MustCompile("^SHA3-([0-9][0-9][0-9]) \\(([^)])\\)[ ]*=[ ]*([0-9A-Fa-f][0-9A-Fa-f]*)$")

// SHA3-XXX (filename) = hex
func parseTagHash(line string) (hash, fname string, algorithm int, err error) {
	if !tagRegexp.MatchString(line) {
		err = fmt.Errorf("bad checksum line")
		return
	}
	// 0 = algorithm
	// 1 = filename
	// 2 = hash
	matches := tagRegexp.FindStringSubmatch(line)
	if len(matches) != 4 {
		err = fmt.Errorf("bad line")
		return
	}
	algorithm, err = strconv.Atoi(matches[1])
	if err != nil {
		return
	}
	if !validAlgorithm(algorithm) {
		err = fmt.Errorf("bad algorithm")
		return
	}
	fname = matches[2]
	if len(fname) == 0 {
		err = fmt.Errorf("bad filename")
		return
	}
	hash = matches[3]
	if len(hash) != algorithm/4 {
		err = fmt.Errorf("bad hash")
		return
	}
	return
}

var normalRegexp = regexp.MustCompile("^([0-9A-Fa-f][0-9A-Fa-f]*)[ ][ ]*(.+)$")

// hex filename
func parseNormalHash(line string) (hash, fname string, algorithm int, err error) {
	if !normalRegexp.MatchString(line) {
		err = fmt.Errorf("bad checksum line")
		return
	}
	matches := normalRegexp.FindStringSubmatch(line)
	if len(matches) != 4 {
		err = fmt.Errorf("bad line")
		return
	}
	hash = matches[1]
	hashlen := len(hash)
	switch hashlen {
	case 224 / 4, 256 / 4, 384 / 4, 512 / 4:
		algorithm = hashlen * 4
	default:
		err = fmt.Errorf("bad hash")
		return
	}
	fname = matches[2]
	if len(fname) == 0 {
		err = fmt.Errorf("bad filename")
		return
	}
	return
}

func parseHash(line string, tag bool) (hash, fname string, algorithm int, err error) {
	if tag {
		return parseTagHash(line)
	} else {
		return parseNormalHash(line)
	}
}

func validAlgorithm(algorithm int) bool {
	switch algorithm {
	case 224, 256, 384, 512:
		return true
	}
	return false
}

func readHashes(hashesFilename string, tag, strict bool) (hashes, filenames []string, algorithms []int) {
	f, err := os.Open(hashesFilename)
	if err != nil {
		dieerr(err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	line := ""
	for {
		part, prefix, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return
		}

		line += string(part)
		if !prefix {
			hash, fname, algorithm, err := parseHash(line, tag)
			if err != nil && strict {
				dieerr(err)
			}
			hashes = append(hashes, hash)
			filenames = append(filenames, fname)
			algorithms = append(algorithms, algorithm)
			line = ""
		}
	}
	return
}

func hashFiles(files []string, algorithm int, tag bool) (err error) {
	if len(files) == 0 {
		err = fmt.Errorf("missing files to check")
		return
	}
	for _, filename := range files {
		hash, err2 := hashFile(filename, algorithm)
		if err2 != nil {
			err = err2
			continue
		}
		if tag {
			fmt.Printf("SHA3-%d (%s) = %s\n", algorithm, filename, hash)
		} else {
			fmt.Printf("%s  %s\n", hash, filename)
		}
	}
	return
}

func checkFiles(checkFilename string, tagFlag, strictFlag, statusFlag bool) error {
	bad := 0
	good := 0
	expectedHexHashes, filenames, algorithms := readHashes(checkFilename, tagFlag, strictFlag)
	for i, filename := range filenames {
		actualHashHex, err := hashFile(filename, algorithms[i])
		if err != nil && strictFlag {
			return err
		}

		actualHash, err2 := hex.DecodeString(actualHashHex)
		if err2 != nil && strictFlag {
			return err2
		}

		expectedHash, err3 := hex.DecodeString(expectedHexHashes[i])
		if err3 != nil && strictFlag {
			return err3
		}
		if err == nil && err2 == nil && err3 == nil && securecompare.Equal(actualHash, expectedHash) {
			if !statusFlag {
				fmt.Printf("%s: OK\n", filename)
			}
			good++
		} else {
			if !statusFlag {
				fmt.Printf("%s: FAILED\n", filename)
			}
			bad++
		}
	}
	if !statusFlag && bad > 0 {
		fmt.Fprintf(os.Stderr, "sha3sum: WARNING %d of %d computed checksums did NOT match\n", bad, (good + bad))
	}
	return nil
}

func flag(flags []string, desc string) *bool {
	return goopt.Flag(flags, []string{}, desc, "")
}

func main() {

	goopt.Summary = "Print or check SHA3 checksums"

	algorithm := goopt.Int([]string{"-a", "--algorithm"}, 256, "224, 256 (default), 384, 512")
	checkFilename := goopt.String([]string{"-c", "--check"}, "", "check SHA3 sums against given list")
	tag := flag([]string{"-t", "--tag"}, "create a BSD-style checksum")
	ver := flag([]string{"-v", "--version"}, "output version information and exit")

	// check options
	stat := flag([]string{"-s", "--status", "-w", "--warn"}, "don't output anything, status code shows success")
	q := flag([]string{"-q", "--quiet"}, "don't print OK for each successfully verified file")
	strict := flag([]string{"--strict"}, "with --check, exit non-zero for any invalid input")

	goopt.Parse(nil)

	if ver != nil && *ver {
		fmt.Println("sha3sum 1.0")
		return
	}

	if algorithm == nil || !validAlgorithm(*algorithm) {
		die("bad algorithm")
	}

	statusFlag := stat != nil && *stat
	quietFlag := q != nil && *q
	strictFlag := strict != nil && *strict
	tagFlag := tag != nil && *tag

	if checkFilename == nil || *checkFilename == "" {
		if statusFlag || quietFlag || strictFlag {
			die("silent, warn, strict and/or quiet can only be used with check")
		}
		files := goopt.Args
		if len(files) == 0 {
			files = []string{"-"}
		}

		if err := hashFiles(files, *algorithm, tagFlag); err != nil {
			os.Exit(1)
		}
	} else {
		if err := checkFiles(*checkFilename, tagFlag, strictFlag, statusFlag); err != nil {
			os.Exit(1)
		}
	}
}
