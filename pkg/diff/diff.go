package diff

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type LineRange struct {
	Start int
	Count int
}

type FileDiff struct {
	OldPath        string
	NewPath        string
	IsNew          bool
	IsDeleted      bool
	AddedLines     []int
	DeletedLines   []int
	ModifiedRanges []LineRange
}

func (f *FileDiff) Path() string {
	if f.NewPath != "" && f.NewPath != "/dev/null" {
		return f.NewPath
	}
	return f.OldPath
}

func ParseUnifiedDiff(r io.Reader) ([]FileDiff, error) {
	scanner := bufio.NewScanner(r)
	var diffs []FileDiff
	var current *FileDiff

	currentNewLine := 0
	currentOldLine := 0

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				current.ModifiedRanges = condenseLineRanges(current.AddedLines)
				diffs = append(diffs, *current)
			}
			current = &FileDiff{}
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				current.OldPath = cleanDiffPath(parts[2])
				current.NewPath = cleanDiffPath(parts[3])
			}
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "new file mode ") {
			current.IsNew = true
			continue
		}
		if strings.HasPrefix(line, "deleted file mode ") {
			current.IsDeleted = true
			continue
		}
		if strings.HasPrefix(line, "--- ") {
			path := strings.TrimPrefix(line, "--- ")
			current.OldPath = cleanDiffPath(path)
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimPrefix(line, "+++ ")
			current.NewPath = cleanDiffPath(path)
			continue
		}

		if strings.HasPrefix(line, "@@ ") {
			newStart, _, oldStart, _, err := parseHunkHeader(line)
			if err == nil {
				currentNewLine = newStart
				currentOldLine = oldStart
			}
			continue
		}

		if len(line) == 0 {
			currentNewLine++
			currentOldLine++
			continue
		}

		prefix := line[0]
		switch prefix {
		case '+':
			current.AddedLines = append(current.AddedLines, currentNewLine)
			currentNewLine++
		case '-':
			current.DeletedLines = append(current.DeletedLines, currentOldLine)
			currentOldLine++
		case ' ':
			currentNewLine++
			currentOldLine++
		}
	}

	if current != nil {
		current.ModifiedRanges = condenseLineRanges(current.AddedLines)
		diffs = append(diffs, *current)
	}

	return diffs, scanner.Err()
}

func cleanDiffPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "/dev/null" {
		return path
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

func parseHunkHeader(line string) (newStart, newCount, oldStart, oldCount int, err error) {
	endIdx := strings.Index(line[3:], " @@")
	if endIdx == -1 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header format: %s", line)
	}
	body := strings.TrimSpace(line[3 : 3+endIdx])
	parts := strings.Split(body, " ")
	if len(parts) < 2 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk parts: %s", line)
	}

	oldSpec := strings.TrimPrefix(parts[0], "-")
	newSpec := strings.TrimPrefix(parts[1], "+")

	oldStart, oldCount = parseRangeSpec(oldSpec)
	newStart, newCount = parseRangeSpec(newSpec)
	return newStart, newCount, oldStart, oldCount, nil
}

func parseRangeSpec(spec string) (start, count int) {
	pieces := strings.Split(spec, ",")
	start, _ = strconv.Atoi(pieces[0])
	if len(pieces) > 1 {
		count, _ = strconv.Atoi(pieces[1])
	} else {
		count = 1
	}
	return start, count
}

func condenseLineRanges(lines []int) []LineRange {
	if len(lines) == 0 {
		return nil
	}

	var ranges []LineRange
	start := lines[0]
	count := 1

	for i := 1; i < len(lines); i++ {
		if lines[i] == lines[i-1]+1 {
			count++
		} else {
			ranges = append(ranges, LineRange{Start: start, Count: count})
			start = lines[i]
			count = 1
		}
	}
	ranges = append(ranges, LineRange{Start: start, Count: count})
	return ranges
}
