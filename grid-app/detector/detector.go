package detector

import (
	"bufio"
	"io"
	"math"
	"regexp"
)

const (
	sampleLines             = 15
	nonDelimiterRegexString = `[[:alnum:]\n\r@\. ]`
)

// New a detector.
func New() Detector {
	return &detector{
		nonDelimiterRegex: regexp.MustCompile(nonDelimiterRegexString),
		sampleLines:       sampleLines,
	}
}

// Detector defines the exposed interface.
type Detector interface {
	DetectDelimiter(reader io.Reader, enclosure byte) []string
	Configure(SampleLines *int, nonDelimiterRegexString *string)
}

// detector is the default implementation of Detector.
type detector struct {
	nonDelimiterRegex *regexp.Regexp
	sampleLines       int
}

// DetectDelimiter finds a slice of delimiter string.
func (d *detector) DetectDelimiter(reader io.Reader, enclosure byte) []string {
	statistics, totalLines := d.sample(reader, d.sampleLines, enclosure)
	var candidates []string
	for _, delimiter := range d.analyze(statistics, totalLines) {
		candidates = append(candidates, string(delimiter))
	}

	return candidates
}

func (d *detector) Configure(sampleLines *int, nonDelimiterRegexString *string) {
	if sampleLines != nil {
		d.sampleLines = *sampleLines
	}
	if nonDelimiterRegexString != nil {
		d.nonDelimiterRegex = regexp.MustCompile(*nonDelimiterRegexString)
	}
}

// sample reads lines and walks through each character, records the frequencies of each candidate delimiter
// at each line(here we call it the 'frequencyTable'). It also returns the actual sampling lines
// because it might be less than sampleLines.
func (d *detector) sample(reader io.Reader, sampleLines int, enclosure byte) (frequencies frequencyTable, actualSampleLines int) {
	bufferedReader := bufio.NewReader(reader)
	frequencies = createFrequencyTable()

	enclosed := false
	actualSampleLines = 1
	var prev, current, next byte
	var err error

	bufSize := 1024
	buf := make([]byte, bufSize)
	n, err := bufferedReader.Read(buf)

	for err == nil {
		for i := 0; i < n; i++ {
			current = buf[i]

			if i > 0 {
				prev = buf[i-1]
			} else {
				prev = byte(0)
			}

			if i < n-1 {
				next = buf[i+1]
			} else {
				next = byte(0)
			}
			if current == enclosure {
				if !enclosed || next != enclosure {
					if enclosed {
						enclosed = false
					} else {
						enclosed = true
					}
				} else {
					i++
				}
			} else if (current == '\n' && prev != '\r' && next != byte(0) && next != '\n' || current == '\r') && !enclosed {
				if actualSampleLines == sampleLines {
					return
				}
				actualSampleLines++
			} else if !enclosed {
				if !d.nonDelimiterRegex.MatchString(string(current)) {
					frequencies.increment(current, actualSampleLines)
				}
			}
		}

		n, err = bufferedReader.Read(buf)
	}

	return
}

// analyze is built based on such an observation: the delimiter must appears
// the same times at each line, usually, it appears more than once. Therefore
// for each delimiter candidate, the deviation of its frequency at each line
// is calculated, if the deviation is 0, it means it appears the same times at
// each sampled line.
func (d *detector) analyze(ft frequencyTable, sampleLine int) []byte {
	mean := func(frequencyOfLine map[int]int, size int) float32 {
		total := 0
		for i := 1; i <= size; i++ {
			if frequency, ok := frequencyOfLine[i]; ok {
				total += frequency
			}
		}
		return float32(total) / float32(size)
	}

	deviation := func(frequencyOfLine map[int]int, size int) float64 {
		average := mean(frequencyOfLine, size)
		var total float64
		for i := 1; i <= size; i++ {
			var frequency float32

			if v, ok := frequencyOfLine[i]; ok {
				frequency = float32(v)
			}

			d := (average - frequency) * (average - frequency)
			total += math.Sqrt(float64(d))
		}

		return total / float64(size)
	}

	var candidates []byte
	for delimiter, frequencyOfLine := range ft {
		if float64(0.0) == deviation(frequencyOfLine, sampleLine) {
			candidates = append(candidates, delimiter)
		}
	}

	return candidates
}

// frequencyTable remembers the frequency of character at each line.
// frequencyTable['.'][11] will get the frequency of char '.' at line 11.
type frequencyTable map[byte]map[int]int

// createFrequencyTable constructs a new frequencyTable.
func createFrequencyTable() frequencyTable {
	return make(map[byte]map[int]int)
}

// increment the frequency for ch at line.
func (f frequencyTable) increment(ch byte, line int) frequencyTable {
	if _, ok := f[ch]; !ok {
		f[ch] = make(map[int]int)
	}

	if _, ok := f[ch][line]; !ok {
		f[ch][line] = 0
	}

	f[ch][line]++

	return f
}
