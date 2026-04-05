package rag

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type LangchainSplitter struct{}

func NewLangchainSplitter() *LangchainSplitter {
	return &LangchainSplitter{}
}

func (s *LangchainSplitter) Split(text string, opts SplitOpts) ([]Chunk, error) {
	if strings.TrimSpace(text) == "" {
		return []Chunk{}, nil
	}
	strategy := strings.TrimSpace(strings.ToLower(opts.Strategy))
	if strategy == "" {
		strategy = "recursive"
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 512
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 0
	}
	if opts.ChunkOverlap >= opts.ChunkSize {
		opts.ChunkOverlap = max(0, opts.ChunkSize-1)
	}

	switch strategy {
	case "fixed":
		return splitByTokenWindows(text, opts.ChunkSize, opts.ChunkOverlap, strategy), nil
	case "sliding":
		return splitByTokenWindows(text, opts.ChunkSize, opts.ChunkOverlap, strategy), nil
	case "semantic":
		chunks := splitSemantic(text, opts.ChunkSize, opts.ChunkOverlap)
		if len(chunks) == 0 {
			chunks = splitRecursive(text, opts.ChunkSize, opts.ChunkOverlap)
		}
		for i := range chunks {
			chunks[i].Metadata.Strategy = strategy
			chunks[i].Metadata.ChunkIndex = i
		}
		return chunks, nil
	case "recursive":
		chunks := splitRecursive(text, opts.ChunkSize, opts.ChunkOverlap)
		for i := range chunks {
			chunks[i].Metadata.Strategy = strategy
			chunks[i].Metadata.ChunkIndex = i
		}
		return chunks, nil
	default:
		return nil, fmt.Errorf("unsupported chunking strategy %q", strategy)
	}
}

type tokenSpan struct {
	start int
	end   int
}

func splitByTokenWindows(text string, chunkSize, overlap int, strategy string) []Chunk {
	tokens := tokenSpans(text)
	if len(tokens) == 0 {
		return []Chunk{}
	}
	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}
	out := make([]Chunk, 0, max(1, len(tokens)/step+1))
	chunkIndex := 0
	for startTok := 0; startTok < len(tokens); startTok += step {
		endTok := min(len(tokens), startTok+chunkSize)
		start := tokens[startTok].start
		end := tokens[endTok-1].end
		content := strings.TrimSpace(text[start:end])
		if content == "" {
			continue
		}
		meta := ChunkMetadata{
			CharStart:  start,
			CharEnd:    end,
			ChunkIndex: chunkIndex,
			Strategy:   strategy,
			PageNumber: pageAtOffset(text, start),
		}
		meta.SectionTitle = sectionAtOffset(text, start)
		out = append(out, Chunk{Content: content, TokenCount: endTok - startTok, Metadata: meta})
		chunkIndex++
		if endTok == len(tokens) {
			break
		}
	}
	return out
}

func splitRecursive(text string, chunkSize, overlap int) []Chunk {
	paras := paragraphSpans(text)
	if len(paras) == 0 {
		return splitByTokenWindows(text, chunkSize, overlap, "recursive")
	}
	out := make([]Chunk, 0)
	for _, p := range paras {
		paragraph := strings.TrimSpace(text[p.start:p.end])
		if paragraph == "" {
			continue
		}
		ptoks := tokenSpans(paragraph)
		if len(ptoks) <= chunkSize {
			start := p.start + ptoks[0].start
			end := p.start + ptoks[len(ptoks)-1].end
			meta := ChunkMetadata{
				CharStart:    start,
				CharEnd:      end,
				ChunkIndex:   len(out),
				Strategy:     "recursive",
				PageNumber:   pageAtOffset(text, start),
				SectionTitle: sectionAtOffset(text, start),
			}
			out = append(out, Chunk{Content: strings.TrimSpace(text[start:end]), TokenCount: len(ptoks), Metadata: meta})
			continue
		}
		part := splitByTokenWindows(paragraph, chunkSize, overlap, "recursive")
		for i := range part {
			part[i].Metadata.CharStart += p.start
			part[i].Metadata.CharEnd += p.start
			part[i].Metadata.ChunkIndex = len(out)
			part[i].Metadata.PageNumber = pageAtOffset(text, part[i].Metadata.CharStart)
			part[i].Metadata.SectionTitle = sectionAtOffset(text, part[i].Metadata.CharStart)
			part[i].Content = strings.TrimSpace(text[part[i].Metadata.CharStart:part[i].Metadata.CharEnd])
			out = append(out, part[i])
		}
	}
	return out
}

func splitSemantic(text string, chunkSize, overlap int) []Chunk {
	headings := headingSpans(text)
	if len(headings) == 0 {
		return nil
	}
	sections := make([]tokenSpan, 0, len(headings))
	for i := range headings {
		start := headings[i].start
		end := len(text)
		if i+1 < len(headings) {
			end = headings[i+1].start
		}
		sections = append(sections, tokenSpan{start: start, end: end})
	}
	out := make([]Chunk, 0)
	for _, sec := range sections {
		sectionText := strings.TrimSpace(text[sec.start:sec.end])
		if sectionText == "" {
			continue
		}
		sectionTitle := firstLine(sectionText)
		sectionChunks := splitByTokenWindows(sectionText, chunkSize, overlap, "semantic")
		for i := range sectionChunks {
			sectionChunks[i].Metadata.CharStart += sec.start
			sectionChunks[i].Metadata.CharEnd += sec.start
			sectionChunks[i].Metadata.ChunkIndex = len(out)
			sectionChunks[i].Metadata.SectionTitle = strings.TrimSpace(strings.TrimPrefix(sectionTitle, "#"))
			sectionChunks[i].Metadata.PageNumber = pageAtOffset(text, sectionChunks[i].Metadata.CharStart)
			sectionChunks[i].Content = strings.TrimSpace(text[sectionChunks[i].Metadata.CharStart:sectionChunks[i].Metadata.CharEnd])
			out = append(out, sectionChunks[i])
		}
	}
	return out
}

func tokenSpans(text string) []tokenSpan {
	out := make([]tokenSpan, 0)
	inToken := false
	start := 0
	for i, r := range text {
		if unicode.IsSpace(r) {
			if inToken {
				out = append(out, tokenSpan{start: start, end: i})
				inToken = false
			}
			continue
		}
		if !inToken {
			inToken = true
			start = i
		}
	}
	if inToken {
		out = append(out, tokenSpan{start: start, end: len(text)})
	}
	return out
}

func paragraphSpans(text string) []tokenSpan {
	parts := strings.Split(text, "\n\n")
	if len(parts) == 0 {
		return nil
	}
	out := make([]tokenSpan, 0, len(parts))
	offset := 0
	for _, p := range parts {
		start := offset
		end := start + len(p)
		if strings.TrimSpace(p) != "" {
			out = append(out, tokenSpan{start: start, end: end})
		}
		offset = end + 2
	}
	return out
}

func headingSpans(text string) []tokenSpan {
	re := regexp.MustCompile(`(?m)^(#{1,3}\s+.+)$`)
	matches := re.FindAllStringIndex(text, -1)
	out := make([]tokenSpan, 0, len(matches))
	for _, m := range matches {
		out = append(out, tokenSpan{start: m[0], end: m[1]})
	}
	return out
}

func firstLine(text string) string {
	idx := strings.IndexByte(text, '\n')
	if idx < 0 {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(text[:idx])
}

func pageAtOffset(text string, offset int) int {
	if offset < 0 {
		offset = 0
	}
	page := 1
	marker := "[Page "
	for i := 0; i < len(text) && i <= offset; i++ {
		if strings.HasPrefix(text[i:], marker) {
			page++
		}
	}
	if page < 1 {
		return 1
	}
	return page
}

func sectionAtOffset(text string, offset int) string {
	re := regexp.MustCompile(`(?m)^(#{1,3}\s+.+)$`)
	all := re.FindAllStringSubmatchIndex(text, -1)
	var title string
	for _, idx := range all {
		if len(idx) < 2 {
			continue
		}
		if idx[0] > offset {
			break
		}
		title = strings.TrimSpace(text[idx[0]:idx[1]])
	}
	return strings.TrimSpace(strings.TrimPrefix(title, "#"))
}
