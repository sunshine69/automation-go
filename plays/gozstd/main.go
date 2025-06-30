package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/mattn/go-isatty"
)

const oneMB = 1 << 20

var (
	version   string // Will hold the version number
	buildTime string // Will hold the build time
)

func printVersionBuildInfo() {
	fmt.Printf("Version: %s\nBuild time: %s\n", version, buildTime)
}

func compressStream(input io.Reader, output io.Writer, compressionLevel int) error {
	encoder, err := zstd.NewWriter(output, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(compressionLevel)))
	if err != nil {
		return fmt.Errorf("failed to create zstd encoder: %w", err)
	}
	defer encoder.Close()

	_, err = io.Copy(encoder, input)
	if err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}

	return nil
}

func compressPart(inputFile string, segmentIndex int, offset [2]int64, compressionLevel int) (outputFile string, err1 error) {
	input, err := os.Open(inputFile)
	if err != nil {
		return "", fmt.Errorf("failed to create openfile: %w", err)
	}
	defer input.Close()

	outputFile = fmt.Sprintf("%d-%s-output-segment.part%d", segmentIndex, filepath.Base(inputFile), segmentIndex)
	output, err := os.Create(outputFile)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer output.Close()

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(compressionLevel)))
	if err != nil {
		return "", fmt.Errorf("failed to create zstd encoder: %w", err)
	}
	defer encoder.Close()

	startOffset, endOffset := offset[0], offset[1]
	buf := make([]byte, oneMB) // 1 MB buffer
	input.Seek(startOffset, 0)
	for {
		n, err := input.Read(buf)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		if n == 0 {
			break
		}

		compressed := encoder.EncodeAll(buf[:n], nil)
		_, err = output.Write(compressed)
		if err != nil {
			return "", fmt.Errorf("failed to write output: %w", err)
		}
		currentOffset, err := input.Seek(0, io.SeekCurrent)
		if err != nil {
			return "", fmt.Errorf("failed to get current offset: %w", err)
		}
		if currentOffset == endOffset { // We need to be sure it ends with 1MB boundary or the last one
			break
		}
		if currentOffset > endOffset {
			panic("[ERROR] I read over the endOffset. That means you pass me index not end in 1MB boundary")
		}
	}

	return outputFile, nil
}

func divmod(numerator, denominator int64) (quotient, remainder int64) {
	quotient = numerator / denominator // integer division, decimals are truncated
	remainder = numerator % denominator
	return
}

func divideFileIntoSegments(fileSize int64, threadCount int) [][2]int64 {
	var segments [][2]int64

	// Convert file size to MB boundaries
	fileSizeMB := (fileSize + oneMB - 1) / oneMB // Round up to the nearest MB

	// Calculate the size of each segment in MB
	segmentSizeMB := fileSizeMB / int64(threadCount)
	remainingMB := fileSizeMB % int64(threadCount)

	// Calculate the start and end offsets for each segment
	var start int64
	for i := 0; i < threadCount; i++ {
		end := start + segmentSizeMB*oneMB
		if remainingMB > 0 {
			end += oneMB
			remainingMB--
		}
		if end > fileSize {
			end = fileSize
		}
		segments = append(segments, [2]int64{start, end})
		start = end
	}

	return segments
}

func calculateSegment(inputFile string, numThreads int) (offset [][2]int64, err1 error) {
	finfo, err := os.Stat(inputFile)
	if err != nil {
		return [][2]int64{}, err
	}
	fSize := finfo.Size()
	return divideFileIntoSegments(fSize, numThreads), nil
}

// FileWithIndex represents a file with its numeric index extracted from its name.
type FileWithIndex struct {
	Index int
	Name  string
}

// concatenateFiles concatenates files based on their numeric index and writes them to the output file.
func concatenateFiles(filenames []string, outputFile string) error {
	var filesWithIndex []FileWithIndex

	// Extract the numeric index from each filename and store it in the filesWithIndex slice.
	for _, filename := range filenames {
		base := filepath.Base(filename)
		parts := strings.SplitN(base, "-", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid filename pattern: %s", filename)
		}

		index, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid index in filename: %s", filename)
		}

		filesWithIndex = append(filesWithIndex, FileWithIndex{Index: index, Name: filename})
	}

	// Sort files based on their numeric index.
	sort.Slice(filesWithIndex, func(i, j int) bool {
		return filesWithIndex[i].Index < filesWithIndex[j].Index
	})

	// Create or truncate the output file.
	out, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer out.Close()

	// Concatenate the contents of each file in order.
	for _, file := range filesWithIndex {
		in, err := os.Open(file.Name)
		if err != nil {
			return fmt.Errorf("failed to open input file %s: %v", file.Name, err)
		}

		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			return fmt.Errorf("failed to write to output file: %v", err)
		}
	}
	for _, file := range filesWithIndex {
		os.Remove(file.Name)
	}
	return nil
}

func compressFileBlock(inputFile, outputFile string, compressionLevel, numThreads int) error {
	offset, err := calculateSegment(inputFile, numThreads)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	outputFileName := make(chan string, numThreads)
	errChan := make(chan error, numThreads)

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outfile, err := compressPart(inputFile, i, offset[i], compressionLevel)
			if err != nil {
				errChan <- err
				outputFileName <- ""
				return
			}
			outputFileName <- outfile
		}(i)
	}

	go func() {
		wg.Wait()
		close(outputFileName)
		close(errChan)
	}()

	fmt.Fprintln(os.Stderr, "Working, please wait ...")
	outputFiles := []string{}
	for fn := range outputFileName {
		outputFiles = append(outputFiles, fn)
	}

	for err := range errChan {
		if err != nil {
			fmt.Println(err.Error())
		}
		panic("[ERROR] some errors see above")
	}

	concatenateFiles(outputFiles, outputFile)
	return nil
}

func decompressFile(input io.Reader, output io.Writer) error {
	decoder, err := zstd.NewReader(input)
	if err != nil {
		return fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	defer decoder.Close()

	_, err = io.Copy(output, decoder)
	if err != nil {
		return fmt.Errorf("failed to decompress data: %w", err)
	}

	return nil
}

func main() {
	// Define flags
	compressMode := flag.Bool("d", false, "Decompress instead of compress")
	outputToStdout := flag.Bool("c", true, "Write output to stdout")
	outputFile := flag.String("o", "", "Output file (default: stdout)")
	compressionLevel := flag.Int("l", 3, "Set compression level (1-19, default: 3)")
	numThreads := flag.Int("T", 2, "Number of threads for compression (default: 2)")
	blockMode := flag.Bool("b", false, "Use block mode for compression. This will use the option -T to utilize more than 2 CPU core. Only benefit if you use compression level higher than 9 otherwise is is not faster in my test but your chances might be vary. You can not use stdin and stdout for this case")
	// With -l 15 the block mode is around three times faster than stream mode with -T 4. However if -l 9 then it is slightly slower (0.3sec)
	// So for low level compression <=9 use stream.

	flag.Usage = func() {
		printVersionBuildInfo()
		flag.PrintDefaults()
	}
	// Parse flags
	flag.Parse()

	// Determine input source
	var input io.Reader = os.Stdin
	if flag.NArg() > 0 {
		inputFile, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Printf("Failed to open input file: %v\n", err)
			os.Exit(1)
		}
		defer inputFile.Close()
		input = inputFile
	}

	// Determine output destination
	output := os.Stdout
	if !*outputToStdout && *outputFile == "" {
		panic("Need to set output file using -o <filename>\n")
	}
	if *outputFile != "" {
		outFile, err := os.Create(*outputFile)
		if err != nil {
			panic("Failed to create output file: " + err.Error() + "\n")
		}
		defer outFile.Close()
		output = outFile
	}

	if isatty.IsTerminal(output.Fd()) {
		fmt.Fprint(os.Stderr, "Cowedly refused to spit out to terminal with binary data...")
		os.Exit(1)
	}
	// Handle compression/decompression
	if *compressMode {
		err := decompressFile(input, output)
		if err != nil {
			fmt.Printf("Decompression failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		if *blockMode {
			if flag.NArg() < 0 || *outputFile == "" {
				panic("[ERROR] Block mode does not support non seekable stream like stdin or stdout. Require option inputfile and -o <outputfile> to work")
			}
			inputFile := flag.Arg(0)

			err := compressFileBlock(inputFile, *outputFile, *compressionLevel, *numThreads)
			if err != nil {
				fmt.Printf("Block mode compression failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			err := compressStream(input, output, *compressionLevel)
			if err != nil {
				fmt.Printf("Stream mode compression failed: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
