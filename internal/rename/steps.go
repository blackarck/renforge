package rename

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"renforge/internal/meta"
)

// Op identifies a rename step operation.
type Op string

const (
	OpRemoveText      Op = "Remove text"
	OpReplaceText     Op = "Replace text"
	OpInsertBeforeExt Op = "Insert before extension"
	OpChangeExt       Op = "Change extension"
	OpAppend          Op = "Append"
	OpPrepend         Op = "Prepend"
	OpRegex           Op = "Regex replace"
	OpCase            Op = "Change case"
	OpCleanup         Op = "Clean up"
	OpNumber          Op = "Number files"
	OpTemplate        Op = "Rename from template"
	OpFrames          Op = "Shift frame numbers"
)

// AllOps lists every operation in display order.
var AllOps = []Op{
	OpReplaceText, OpRemoveText, OpRegex, OpPrepend, OpAppend, OpInsertBeforeExt,
	OpChangeExt, OpCase, OpCleanup, OpNumber, OpTemplate, OpFrames,
}

// Options for the "Change case" step (stored in Step.A).
const (
	CaseLower    = "lowercase"
	CaseUpper    = "UPPERCASE"
	CaseTitle    = "Title Case"
	CaseSentence = "Sentence case"
	CaseSnake    = "snake_case"
	CaseKebab    = "kebab-case"
	CaseExtLower = "lowercase extension"
	CaseExtUpper = "UPPERCASE extension"
)

// Options for the "Clean up" step (stored in Step.A).
const (
	CleanSpaces  = "Collapse repeated spaces"
	CleanTrim    = "Trim spaces around name"
	CleanAccents = "Remove accents"
	CleanDots    = "Dots and underscores to spaces"
	CleanCopies  = "Remove copy markers like (1) or - Copy"
)

// CaseOptions and CleanupOptions list the choices in display order.
var (
	CaseOptions    = []string{CaseLower, CaseUpper, CaseTitle, CaseSentence, CaseSnake, CaseKebab, CaseExtLower, CaseExtUpper}
	CleanupOptions = []string{CleanSpaces, CleanTrim, CleanAccents, CleanDots, CleanCopies}
)

// Options returns the fixed choices for ops whose A field is a pick-list, or
// nil when A is free text.
func Options(op Op) []string {
	switch op {
	case OpCase:
		return CaseOptions
	case OpCleanup:
		return CleanupOptions
	}
	return nil
}

// Step is one stage of the rename pipeline.
type Step struct {
	ID       int    `json:"-"`
	Op       Op     `json:"op"`
	A        string `json:"a,omitempty"`
	B        string `json:"b,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	// Flag restarts the numbering in every folder (Number files / template).
	Flag bool `json:"flag,omitempty"`
}

var (
	reCache    sync.Map // pattern -> *regexCacheEntry
	reSpaces   = regexp.MustCompile(`\s{2,}`)
	reCopySfx  = regexp.MustCompile(`(?i)(\s*\(\d+\)|\s*-\s*copy(\s*\(\d+\))?)\s*$`)
	reCopyPfx  = regexp.MustCompile(`(?i)^copy of\s+`)
	reTemplate = regexp.MustCompile(`\{([a-z]+)(?::([^{}]*))?\}`)
)

type regexCacheEntry struct {
	re  *regexp.Regexp
	err error
}

// CompileRegex compiles (and caches) a pattern.
func CompileRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := reCache.Load(pattern); ok {
		e := v.(*regexCacheEntry)
		return e.re, e.err
	}
	re, err := regexp.Compile(pattern)
	reCache.Store(pattern, &regexCacheEntry{re, err})
	return re, err
}

// templateTokens are the tokens understood by the template steps.
var templateTokens = map[string]meta.Level{
	"n": meta.None, "name": meta.None, "parent": meta.None,
	"modified": meta.Stat,
	"date":     meta.EXIF, "camera": meta.EXIF, "make": meta.EXIF, "lens": meta.EXIF, "iso": meta.EXIF,
}

const (
	defaultNumberTemplate   = "{name}_{n:3}"
	defaultTemplateTemplate = "{date:YYYY-MM-DD}_{n:3}"
)

func effectiveTemplate(s Step) string {
	if s.A != "" {
		return s.A
	}
	if s.Op == OpTemplate {
		return defaultTemplateTemplate
	}
	return defaultNumberTemplate
}

// MetaNeeds returns how much file metadata the enabled steps need before the
// names can be computed (None if no step reads size, dates or EXIF).
func MetaNeeds(steps []Step) meta.Level {
	need := meta.None
	for _, s := range steps {
		if s.Disabled || (s.Op != OpNumber && s.Op != OpTemplate) {
			continue
		}
		for _, m := range reTemplate.FindAllStringSubmatch(effectiveTemplate(s), -1) {
			if lvl := templateTokens[m[1]]; lvl > need {
				need = lvl
			}
		}
	}
	return need
}

// ParseSequence parses the "start" or "start,step" setting of the numbering
// steps. An empty setting means 1,1.
func ParseSequence(s string) (start, step int, err error) {
	start, step = 1, 1
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	parts := strings.Split(s, ",")
	if len(parts) > 2 {
		return 1, 1, fmt.Errorf("use start or start,step (e.g. 1 or 100,10)")
	}
	if start, err = strconv.Atoi(strings.TrimSpace(parts[0])); err != nil {
		return 1, 1, fmt.Errorf("start must be a whole number")
	}
	if len(parts) == 2 {
		if step, err = strconv.Atoi(strings.TrimSpace(parts[1])); err != nil {
			return 1, 1, fmt.Errorf("step must be a whole number")
		}
		if step == 0 {
			return 1, 1, fmt.Errorf("step cannot be 0")
		}
	}
	return start, step, nil
}

// ValidateStep returns a human-readable problem with a step's settings, or "".
func ValidateStep(s Step) string {
	switch s.Op {
	case OpRegex:
		if s.A == "" {
			return ""
		}
		if _, err := CompileRegex(s.A); err != nil {
			return "Invalid regular expression: " + strings.TrimPrefix(err.Error(), "error parsing regexp: ")
		}
	case OpNumber, OpTemplate:
		if _, _, err := ParseSequence(s.B); err != nil {
			return "Start/step: " + err.Error()
		}
		t := effectiveTemplate(s)
		for _, m := range reTemplate.FindAllStringSubmatch(t, -1) {
			if _, ok := templateTokens[m[1]]; !ok {
				return fmt.Sprintf("Unknown token {%s}. Use {n}, {name}, {parent}, {date}, {modified}, {camera}, {make}, {lens}, {iso}.", m[1])
			}
		}
		if !strings.Contains(t, "{n") {
			return "The template has no {n}, so files taken at the same moment would get the same name."
		}
	case OpFrames:
		if _, err := parseOffset(s.A); err != nil {
			return "Offset must be a whole number such as +1000 or -1."
		}
		if p := strings.TrimSpace(s.B); p != "" {
			if v, err := strconv.Atoi(p); err != nil || v < 1 || v > 12 {
				return "Padding must be a number from 1 to 12 (or empty to keep the current padding)."
			}
		}
	}
	return ""
}

// FileCtx is the per-file information steps may use.
type FileCtx struct {
	Path        string
	Index       int // position in the whole batch (0-based)
	FolderIndex int // position within the file's folder (0-based)
	Meta        meta.Info
	HasMeta     bool
}

// ApplySteps runs every enabled step over a file name (not a path) with no
// file context.
func ApplySteps(original string, steps []Step) string { return ApplyStepsAt(original, steps, 0) }

// ApplyStepsAt is ApplySteps with a batch position for the numbering steps.
func ApplyStepsAt(original string, steps []Step, idx int) string {
	return ApplyStepsCtx(original, steps, FileCtx{Index: idx, FolderIndex: idx})
}

// ApplyStepsCtx runs every enabled step, in order. The result is NOT trimmed:
// stray leading/trailing spaces are reported by InvalidReason.
func ApplyStepsCtx(original string, steps []Step, ctx FileCtx) string {
	name := original
	for _, s := range steps {
		if s.Disabled {
			continue
		}
		switch s.Op {
		case OpRemoveText:
			if s.A != "" {
				name = strings.ReplaceAll(name, s.A, "")
			}
		case OpReplaceText:
			if s.A != "" {
				name = strings.ReplaceAll(name, s.A, s.B)
			}
		case OpRegex:
			if s.A != "" {
				if re, err := CompileRegex(s.A); err == nil {
					name = re.ReplaceAllString(name, s.B)
				}
			}
		case OpInsertBeforeExt, OpAppend:
			base, ext := SplitExt(name)
			name = base + s.A + ext
		case OpPrepend:
			name = s.A + name
		case OpChangeExt:
			base, _ := SplitExt(name)
			newExt := strings.TrimSpace(s.A)
			switch {
			case newExt == "":
				name = base
			case strings.HasPrefix(newExt, "."):
				name = base + newExt
			default:
				name = base + "." + newExt
			}
		case OpCase:
			name = changeCase(name, s.A)
		case OpCleanup:
			name = cleanup(name, s.A)
		case OpNumber, OpTemplate:
			name = renderTemplate(name, s, ctx)
		case OpFrames:
			name = shiftFrames(name, s.A, s.B)
		}
	}
	return name
}

// PreviewNames returns the new name of each file. A file's position in the
// slice is its sequence index; metadata is taken from the cache only (see
// meta.Ensure), so this never touches the disk.
func PreviewNames(files []string, steps []Step) []string {
	need := MetaNeeds(steps)
	perFolder := map[string]int{}
	out := make([]string, len(files))
	for i, p := range files {
		dir := filepath.Dir(p)
		ctx := FileCtx{Path: p, Index: i, FolderIndex: perFolder[dir]}
		perFolder[dir]++
		if need > meta.None {
			ctx.Meta, ctx.HasMeta = meta.Cached(p, need)
		}
		out[i] = ApplyStepsCtx(filepath.Base(p), steps, ctx)
	}
	return out
}

var dateLayout = strings.NewReplacer("YYYY", "2006", "YY", "06", "MM", "01", "DD", "02", "HH", "15", "mm", "04", "ss", "05")

func formatTime(t time.Time, format string) string {
	if t.IsZero() {
		return "…"
	}
	if format == "" {
		format = "YYYY-MM-DD"
	}
	return t.Format(dateLayout.Replace(format))
}

func renderTemplate(name string, s Step, ctx FileCtx) string {
	start, step, _ := ParseSequence(s.B)
	idx := ctx.Index
	if s.Flag {
		idx = ctx.FolderIndex
	}
	n := start + idx*step
	base, ext := SplitExt(name)

	out := reTemplate.ReplaceAllStringFunc(effectiveTemplate(s), func(tok string) string {
		m := reTemplate.FindStringSubmatch(tok)
		key, arg := m[1], m[2]
		orUnknown := func(v string) string {
			if v == "" {
				if ctx.HasMeta {
					return "unknown"
				}
				return "…"
			}
			return v
		}
		switch key {
		case "n":
			pad, _ := strconv.Atoi(arg)
			return fmt.Sprintf("%0*d", pad, n)
		case "name":
			return base
		case "parent":
			if ctx.Path == "" {
				return ""
			}
			return filepath.Base(filepath.Dir(ctx.Path))
		case "date":
			if !ctx.HasMeta {
				return "…"
			}
			return formatTime(ctx.Meta.Date(), arg)
		case "modified":
			if !ctx.HasMeta {
				return "…"
			}
			return formatTime(ctx.Meta.ModTime, arg)
		case "camera":
			return orUnknown(ctx.Meta.Model)
		case "make":
			return orUnknown(ctx.Meta.Make)
		case "lens":
			return orUnknown(ctx.Meta.Lens)
		case "iso":
			if ctx.Meta.ISO == 0 {
				return orUnknown("")
			}
			return strconv.Itoa(ctx.Meta.ISO)
		}
		return tok
	})
	return out + ext
}

var reFrame = regexp.MustCompile(`^(.*?)(\d+)(\D*)$`)

func parseOffset(s string) (int, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "+")
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// shiftFrames adds an offset to the last number in the name (the frame number
// in "shot.0042.exr") and optionally re-pads it.
func shiftFrames(name, offset, pad string) string {
	base, ext := SplitExt(name)
	m := reFrame.FindStringSubmatch(base)
	if m == nil {
		return name
	}
	off, err := parseOffset(offset)
	if err != nil {
		return name
	}
	frame, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || frame+int64(off) < 0 {
		return name
	}
	width := len(m[2])
	if p, err := strconv.Atoi(strings.TrimSpace(pad)); err == nil && p > 0 {
		width = p
	}
	return m[1] + fmt.Sprintf("%0*d", width, frame+int64(off)) + m[3] + ext
}

func changeCase(name, mode string) string {
	base, ext := SplitExt(name)
	switch mode {
	case CaseLower:
		base = strings.ToLower(base)
	case CaseUpper:
		base = strings.ToUpper(base)
	case CaseTitle:
		base = titleCase(base)
	case CaseSentence:
		base = sentenceCase(base)
	case CaseSnake, CaseKebab:
		if words := splitWords(base); len(words) > 0 {
			sep := "_"
			if mode == CaseKebab {
				sep = "-"
			}
			for i := range words {
				words[i] = strings.ToLower(words[i])
			}
			base = strings.Join(words, sep)
		}
	case CaseExtLower:
		ext = strings.ToLower(ext)
	case CaseExtUpper:
		ext = strings.ToUpper(ext)
	}
	return base + ext
}

func titleCase(s string) string {
	var b strings.Builder
	inWord := false
	for _, r := range s {
		w := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\''
		switch {
		case w && !inWord:
			b.WriteRune(unicode.ToUpper(r))
		case w:
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
		inWord = w
	}
	return b.String()
}

func sentenceCase(s string) string {
	rs := []rune(strings.ToLower(s))
	for i, r := range rs {
		if unicode.IsLetter(r) {
			rs[i] = unicode.ToUpper(r)
			break
		}
	}
	return string(rs)
}

// splitWords splits on non-alphanumerics and on lower→Upper camelCase edges.
func splitWords(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	var prev rune
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			prev = 0
			continue
		}
		if unicode.IsUpper(r) && prev != 0 && unicode.IsLower(prev) {
			flush()
		}
		cur = append(cur, r)
		prev = r
	}
	flush()
	return words
}

func cleanup(name, mode string) string {
	base, ext := SplitExt(name)
	switch mode {
	case CleanSpaces:
		return reSpaces.ReplaceAllString(name, " ")
	case CleanTrim:
		base = strings.TrimSpace(base)
	case CleanAccents:
		var b strings.Builder
		for _, r := range norm.NFD.String(base) {
			if !unicode.Is(unicode.Mn, r) {
				b.WriteRune(r)
			}
		}
		base = norm.NFC.String(b.String())
	case CleanDots:
		base = strings.NewReplacer(".", " ", "_", " ").Replace(base)
	case CleanCopies:
		base = reCopySfx.ReplaceAllString(base, "")
		base = reCopyPfx.ReplaceAllString(base, "")
	}
	return base + ext
}
