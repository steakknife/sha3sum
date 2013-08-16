package main

import (
    "github.com/droundy/goopt"
    "github.com/steakknife/keccak"
    "github.com/steakknife/securecompare"
    "bufio"
    "encoding/hex"
    "fmt"
    "hash"
    "io"
    "os"
    "regexp"
    "strconv"
)

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
// -b binary (windows default)
// -t text (default)
//
// -c check
//
// -s silent
//

func die(msg string) {
    fmt.Fprintln(os.Stderr, msg)
    os.Exit(1)
}

func dieerr(err error) {
    die(fmt.Sprint(err))
}

const BUF_SIZE = 256*1024

var windows bool = func() bool {
    return os.Getenv("WINDIR") != ""
}()

func hashFile(filename string, algorithm int, portable, binary bool) (result string, err error) {
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
        case 224: h = keccak.New224()
        case 256: h = keccak.New256()
        case 384: h = keccak.New384()
        case 512: h = keccak.New512()
    }

    if binary || portable || !windows { // binary || portable
        buf := make([]byte, BUF_SIZE)
        for {
            n, err2 := f.Read(buf)
            if err2 != nil {
                if err2 != io.EOF {
                    err = err2
                }
                break
            }
            if n > 0 {
                h.Write(buf[:n])
            }
        }
    } else { // text on windows
        reader := bufio.NewReader(f)
        for {
            line, err2 := reader.ReadString('\n')
            if err2 != nil {
                if err2 != io.EOF {
                    err = err2
                }
                break
            }
            linelen := len(line)
            if (line[:linelen] == "\r") {
                line = line[:linelen-1]
            }
            h.Write([]byte(line))
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
func parseTagHash(line string) (hash, fname string, algorithm int, portable, binary bool, err error) {
    if ! tagRegexp.MatchString(line) {
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
    if ! validAlgorithm(algorithm) {
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

var normalRegexp = regexp.MustCompile("^([0-9A-Fa-f][0-9A-Fa-f]*)[ ][ ]*([*?])?(.+)$")

// hex filename
func parseNormalHash(line string) (hash, fname string, algorithm int, portable, binary bool, err error) {
    if ! normalRegexp.MatchString(line) {
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
        case 224/4, 256/4, 384/4, 512/4: algorithm = hashlen*4
        default:
            err = fmt.Errorf("bad hash")
            return
    }
    portable = (matches[2] == "?")
    binary = (matches[2] == "*")
    fname = matches[3]
    if len(fname) == 0 {
        err = fmt.Errorf("bad filename")
        return
    }
    return
}

func parseHash(line string, tag bool) (hash, fname string, algorithm int, portable, binary bool, err error) {
    if tag {
        return parseTagHash(line)
    } else {
        return parseNormalHash(line)
    }
}

func validAlgorithm(algorithm int) bool {
    switch algorithm {
        case 224, 256, 384, 512: return true
        default:                 return false
    }
}

func readHashes(hashesFilename string, tag, strict bool) (hashes, filenames []string, algorithms []int, portables, binaries []bool) {
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
        if ! prefix {
            hash, fname, algorithm, portable, binary, err := parseHash(line, tag)
            if err != nil && strict {
                dieerr(err)
            }
            hashes = append(hashes, hash)
            filenames = append(filenames, fname)
            algorithms = append(algorithms, algorithm)
            portables = append(portables, portable)
            binaries = append(binaries, binary)
            line = ""
        }
    }
    return
}

func hashFiles(files []string, algorithm int, portable, binary, tag bool) (err error) {
    if len(files) == 0 {
        err = fmt.Errorf("missing files to check")
        return
    }
    for _, filename := range files {
        hash, err2 := hashFile(filename, algorithm, portable, binary)
        if err2 != nil {
            err = err2
            continue
        }
        if tag{
            fmt.Printf("SHA3-%d (%s) = %s\n", algorithm, filename, hash)
        } else {
            fmt.Printf("%s  ", hash)
            if portable {
                fmt.Print("?")
            } else if binary {
                fmt.Print("*")
            }
            fmt.Println(filename)
        }
    }
    return
}

func checkFiles(checkFilename string, binaryFlag, portableFlag, tagFlag, strictFlag, statusFlag bool) error {
    fmt.Println("checking ", checkFilename)
    bad := 0
    good := 0
    expectedHexHashes, filenames, algorithms, portables, binaries := readHashes(checkFilename, tagFlag, strictFlag)
    fmt.Println("checking files" , filenames)
    for i, filename := range filenames {
        actualHashHex, err := hashFile(filename, algorithms[i], portableFlag || portables[i], binaryFlag || binaries[i])
        if err != nil {
            if strictFlag {
                return err
            } else {
                continue
            }
        }

        actualHash, err := hex.DecodeString(actualHashHex)
        if err != nil {
            if strictFlag {
                return err
            } else {
                continue
            }
        }
        expectedHash, err := hex.DecodeString(expectedHexHashes[i])
        if err != nil {
            if strictFlag {
                return err
            } else {
                continue
            }
        }
        if securecompare.Equal(actualHash, expectedHash) {
            if ! statusFlag {
                fmt.Printf("%s: OK\n", filename)
            }
            good++
        } else {
            if ! statusFlag {
                fmt.Printf("%s: FAILED\n", filename)
            }
            bad++
        }
    }
    if ! statusFlag && bad > 0 {
        fmt.Fprintf(os.Stderr, "sha3sum: WARNING %d of %d computed checksums did NOT match\n", bad, (good+bad))
    }
    return nil
}

func main() {
    goopt.Summary = "Print or check SHA3 checksums"

    algorithm := goopt.Int([]string{"-a", "--algorithm"}, 256, "224, 256 (default), 384, 512")
    binary    := goopt.Flag([]string{"-b", "--binary"},  []string{}, "read files in binary mode (default on DOS/Windows)", "")
    check     := goopt.String([]string{"-c", "--check"}, "", "check SHA3 sums against given list")
    portable  := goopt.Flag([]string{"-p", "--portable"}, []string{}, "read files in portable mode (same digest on Windows/Unix/Mac)", "")
    text      := goopt.Flag([]string{"-t", "--text"}, []string{}, "read files in text mode (default)", "")

    tag       := goopt.Flag([]string{"--tag"}, []string{}, "create a BSD-style checksum", "")


    // check options

    status    := goopt.Flag([]string{"-s", "--status", "-w", "--warn"}, []string{}, "don't output anything, status code shows success", "")
    quiet     := goopt.Flag([]string{"-q", "--quiet"}, []string{}, "don't print OK for each successfully verified file", "")
    strict    := goopt.Flag([]string{"--strict"}, []string{}, "with --check, exit non-zero for any invalid input", "")

    version   := goopt.Flag([]string{"-v", "--version"}, []string{}, "output version information and exit", "")

    goopt.Parse(nil)

    if algorithm == nil && ! validAlgorithm(*algorithm) {
        die("bad algorithm")
    }


    binaryFlag := (binary != nil && *binary)
    portableFlag := (portable != nil && *portable)
    textFlag := (text != nil && *text)

    tagFlag := (tag != nil && *tag)

    statusFlag := (status != nil && *status)
    quietFlag := (quiet != nil && *quiet)
    strictFlag := (strict != nil && *strict)

    versionFlag := (version != nil && *version)

    if textFlag && binaryFlag {
        die("cannot specify both text and binary")
    }

    if (check == nil && *check != "") && (statusFlag || quietFlag || strictFlag) {
        die("silent, warn, strict and/or quiet can only be used with check")
    }

    var files []string
    if len(goopt.Args) == 0 {
        files = []string{"-"}
    } else {
        files = goopt.Args
    }

    if versionFlag {
        fmt.Println("sha3sum 1.0")
        return
    }

    var err error
    if *check == "" {
        err = hashFiles(files, *algorithm, portableFlag, binaryFlag, tagFlag)
    } else {
        checkFilename := *check
        err = checkFiles(checkFilename, binaryFlag, portableFlag, tagFlag, strictFlag, statusFlag)
    }
    if err != nil {
        os.Exit(1)
    }
}