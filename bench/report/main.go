package main

import (
	"bufio"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type BenchKey struct {
	Version string
	Size    string
}

type Stats struct {
	NsOp     []float64
	BytesOp  []float64
	AllocsOp []float64
}

func (s *Stats) median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	cp := make([]float64, len(v))
	copy(cp, v)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 0 {
		return (cp[n/2-1] + cp[n/2]) / 2
	}
	return cp[n/2]
}

func (s *Stats) MedianNsOp() float64     { return s.median(s.NsOp) }
func (s *Stats) MedianBytesOp() float64  { return s.median(s.BytesOp) }
func (s *Stats) MedianAllocsOp() float64 { return s.median(s.AllocsOp) }

type Dataset map[BenchKey]*Stats

var lineRe = regexp.MustCompile(
	`^Benchmark(\w+)/(\w+)/(\w+)-\d+\s+\d+\s+([\d.]+) ns/op(?:\s+[\d.]+ \S+)?\s+([\d.]+) B/op\s+([\d.]+) allocs/op`)

func parseFile(path string) (Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ds := make(Dataset)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := BenchKey{Version: m[2], Size: m[3]}
		nsOp, _ := strconv.ParseFloat(m[4], 64)
		bytesOp, _ := strconv.ParseFloat(m[5], 64)
		allocsOp, _ := strconv.ParseFloat(m[6], 64)

		if ds[key] == nil {
			ds[key] = &Stats{}
		}
		ds[key].NsOp = append(ds[key].NsOp, nsOp)
		ds[key].BytesOp = append(ds[key].BytesOp, bytesOp)
		ds[key].AllocsOp = append(ds[key].AllocsOp, allocsOp)
	}
	return ds, scanner.Err()
}

var versionOrder = []string{"V1_Baseline", "V2_Pools", "V3_Flat", "V4_Sonic"}
var versionLabel = map[string]string{
	"V1_Baseline": "V1 Baseline",
	"V2_Pools":    "V2 Pools",
	"V3_Flat":     "V3 Flat structs",
	"V4_Sonic":    "V4 Sonic",
}
var sizeOrder = []string{"100", "1k", "10k", "50k"}

var versionColors = map[string]string{
	"V1_Baseline": "rgba(0, 209, 255, 0.92)",
	"V2_Pools":    "rgba(255, 65, 54, 0.92)",
	"V3_Flat":     "rgba(155, 48, 255, 0.92)",
	"V4_Sonic":    "rgba(50, 205, 50, 0.92)",
}

type GroupedChart struct {
	Labels   []string
	Datasets []struct {
		Label  string
		Color  string
		Values []float64
	}
}

type VersionChart struct {
	Labels []string
	Values []float64
	Colors []string
}

type LatencyPanel struct {
	Title    string
	CanvasID string
	Labels   []string
	Values   []float64
	Colors   []string
}

type DeltaPctPanel struct {
	Title    string
	CanvasID string
	Labels   []string
	Values   []float64
	Colors   []string
}

type EnvInfo struct {
	GeneratedAt  string
	GoVersion    string
	GOOS         string
	GOARCH       string
	NumCPU       int
	GOMAXPROCS   int
	SonicVersion string
	Hostname     string
}

func collectEnv() EnvInfo {
	host, _ := os.Hostname()
	return EnvInfo{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		GoVersion:    runtime.Version(),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		NumCPU:       runtime.NumCPU(),
		GOMAXPROCS:   runtime.GOMAXPROCS(0),
		SonicVersion: sonicVersion(),
		Hostname:     host,
	}
}

func sonicVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "—"
	}
	for _, m := range info.Deps {
		if m.Path == "github.com/bytedance/sonic" {
			return m.Version
		}
	}
	return "—"
}

func toJS(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			parts[i] = "null"
		} else {
			parts[i] = strconv.FormatFloat(f, 'f', 2, 64)
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func toJSStrings(v []string) string {
	quoted := make([]string, len(v))
	for i, s := range v {
		quoted[i] = `"` + s + `"`
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func buildGroupedByVersion(ds Dataset, metric func(*Stats) float64, sizes []string) GroupedChart {
	var gc GroupedChart
	gc.Labels = sizes
	for _, ver := range versionOrder {
		var row struct {
			Label  string
			Color  string
			Values []float64
		}
		row.Label = versionLabel[ver]
		row.Color = versionColors[ver]
		for _, sz := range sizes {
			key := BenchKey{Version: ver, Size: sz}
			if s, ok := ds[key]; ok {
				row.Values = append(row.Values, metric(s))
			} else {
				row.Values = append(row.Values, 0)
			}
		}
		gc.Datasets = append(gc.Datasets, row)
	}
	return gc
}

func buildVersionChart(ds Dataset, metric func(*Stats) float64, size string) VersionChart {
	var vc VersionChart
	for _, ver := range versionOrder {
		vc.Labels = append(vc.Labels, versionLabel[ver])
		vc.Colors = append(vc.Colors, versionColors[ver])
		key := BenchKey{Version: ver, Size: size}
		if s, ok := ds[key]; ok {
			vc.Values = append(vc.Values, metric(s))
		} else {
			vc.Values = append(vc.Values, 0)
		}
	}
	return vc
}

func sizeTitle(sz string) string {
	switch sz {
	case "100":
		return "100 событий"
	case "1k":
		return "1k событий"
	case "10k":
		return "10k событий"
	case "50k":
		return "50k событий"
	default:
		return sz + " событий"
	}
}

func buildLatencyPanels(ds Dataset, sizes []string) []LatencyPanel {
	nsOp := func(s *Stats) float64 { return s.MedianNsOp() }
	out := make([]LatencyPanel, 0, len(sizes))
	for i, sz := range sizes {
		vc := buildVersionChart(ds, nsOp, sz)
		out = append(out, LatencyPanel{
			Title:    sizeTitle(sz),
			CanvasID: fmt.Sprintf("nsPanel%d", i),
			Labels:   vc.Labels,
			Values:   vc.Values,
			Colors:   vc.Colors,
		})
	}
	return out
}

func buildDeltaNsPct1k(a, b Dataset) *DeltaPctPanel {
	var p DeltaPctPanel
	p.Title = "Δ ms/op (второй относительно первого), %"
	p.CanvasID = "deltaNs1k"
	for _, ver := range versionOrder {
		ka := BenchKey{Version: ver, Size: "1k"}
		kb := ka
		sa, oka := a[ka]
		sb, okb := b[kb]
		if !oka || !okb || sa.MedianNsOp() == 0 {
			continue
		}
		d := (sb.MedianNsOp() - sa.MedianNsOp()) / sa.MedianNsOp() * 100
		p.Labels = append(p.Labels, versionLabel[ver])
		p.Values = append(p.Values, d)
		p.Colors = append(p.Colors, deltaPctBarColor(d))
	}
	if len(p.Labels) == 0 {
		return nil
	}
	return &p
}

func deltaPctBarColor(deltaPct float64) string {
	if deltaPct < -0.5 {
		return "rgba(0, 209, 255, 0.92)"
	}
	if deltaPct > 0.5 {
		return "rgba(255, 65, 54, 0.92)"
	}
	return "rgba(155, 48, 255, 0.45)"
}

type TemplateData struct {
	Env           EnvInfo
	SourceFile    string
	PrimaryLabel  string
	CmpFile       string
	CmpLabel      string
	HasCompare    bool
	ChartNote     string
	LatencyPanels []LatencyPanel
	DeltaNs1k     *DeltaPctPanel
	AllocsGrouped GroupedChart
	BytesGrouped  GroupedChart
	Allocs1k      VersionChart
	Bytes1k       VersionChart
	TableRows     []TableRow
}

type TableRow struct {
	Version     string
	Size        string
	MsOp        string
	AllocsOp    string
	KbOp        string
	HasCompare  bool
	MsOpB       string
	AllocsOpB   string
	KbOpB       string
	DeltaMs     string
	DeltaAllocs string
	DeltaKb     string
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'f', 0, 64) }
func nsToMs(ns float64) string  { return strconv.FormatFloat(ns/1e6, 'f', 3, 64) }
func bToKb(b float64) string    { return strconv.FormatFloat(b/1024, 'f', 1, 64) }

func pctDelta(newVal, oldVal float64) float64 {
	if oldVal == 0 {
		return 0
	}
	return (newVal - oldVal) / oldVal * 100
}

func formatPct(p float64) string {
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return "—"
	}
	s := "+"
	if p < 0 {
		s = ""
	}
	return fmt.Sprintf("%s%.2f%%", s, p)
}

func activeSizesFrom(ds Dataset) []string {
	out := []string{}
	for _, sz := range sizeOrder {
		for _, ver := range versionOrder {
			if _, ok := ds[BenchKey{Version: ver, Size: sz}]; ok {
				out = append(out, sz)
				break
			}
		}
	}
	return out
}

func buildTemplateData(primary, cmp Dataset, primaryPath, cmpPath string, env EnvInfo) TemplateData {
	activeSizes := activeSizesFrom(primary)
	allocsOp := func(s *Stats) float64 { return s.MedianAllocsOp() }
	bytesOp := func(s *Stats) float64 { return s.MedianBytesOp() }

	primaryLabel := filepath.Base(primaryPath)
	cmpLabel := filepath.Base(cmpPath)
	hasCmp := len(cmp) > 0 && cmpPath != ""

	td := TemplateData{
		Env:           env,
		SourceFile:    primaryPath,
		PrimaryLabel:  primaryLabel,
		CmpFile:       cmpPath,
		CmpLabel:      cmpLabel,
		HasCompare:    hasCmp,
		ChartNote:     "Графики построены по первому прогону («Прогон A»).",
		LatencyPanels: buildLatencyPanels(primary, activeSizes),
		AllocsGrouped: buildGroupedByVersion(primary, allocsOp, activeSizes),
		BytesGrouped:  buildGroupedByVersion(primary, bytesOp, activeSizes),
		Allocs1k:      buildVersionChart(primary, allocsOp, "1k"),
		Bytes1k:       buildVersionChart(primary, bytesOp, "1k"),
	}

	if hasCmp {
		td.DeltaNs1k = buildDeltaNsPct1k(primary, cmp)
	}

	for _, ver := range versionOrder {
		for _, sz := range activeSizes {
			key := BenchKey{Version: ver, Size: sz}
			sa, ok := primary[key]
			if !ok {
				continue
			}
			row := TableRow{
				Version:  versionLabel[ver],
				Size:     sz,
				MsOp:     nsToMs(sa.MedianNsOp()),
				AllocsOp: fmtFloat(sa.MedianAllocsOp()),
				KbOp:     bToKb(sa.MedianBytesOp()),
			}
			if hasCmp {
				row.HasCompare = true
				if sb, okb := cmp[key]; okb {
					row.MsOpB = nsToMs(sb.MedianNsOp())
					row.AllocsOpB = fmtFloat(sb.MedianAllocsOp())
					row.KbOpB = bToKb(sb.MedianBytesOp())
					dM := pctDelta(sb.MedianNsOp(), sa.MedianNsOp())
					dA := pctDelta(sb.MedianAllocsOp(), sa.MedianAllocsOp())
					dK := pctDelta(sb.MedianBytesOp(), sa.MedianBytesOp())
					row.DeltaMs = formatPct(dM)
					row.DeltaAllocs = formatPct(dA)
					row.DeltaKb = formatPct(dK)
				} else {
					row.MsOpB = "—"
					row.AllocsOpB = "—"
					row.KbOpB = "—"
					row.DeltaMs = "—"
					row.DeltaAllocs = "—"
					row.DeltaKb = "—"
				}
			}
			td.TableRows = append(td.TableRows, row)
		}
	}

	return td
}

//go:embed report.html.tmpl
var reportTemplate string

func main() {
	inFile := flag.String("in", "results/bench_all_nopgo.txt", "первый прогон (основа графиков и левые колонки)")
	cmpFile := flag.String("cmp", "", "второй прогон для сравнения (опционально)")
	outFile := flag.String("out", "results/report.html", "выходной HTML")
	flag.Parse()

	primary, err := parseFile(*inFile)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}
	fmt.Printf("Parsed %d benchmark entries from %s\n", len(primary), *inFile)

	var cmp Dataset
	if *cmpFile != "" {
		cmp, err = parseFile(*cmpFile)
		if err != nil {
			log.Fatalf("parse -cmp: %v", err)
		}
		fmt.Printf("Parsed %d benchmark entries from %s (compare)\n", len(cmp), *cmpFile)
	}

	td := buildTemplateData(primary, cmp, *inFile, *cmpFile, collectEnv())

	funcMap := template.FuncMap{
		"toJS":        toJS,
		"toJSStrings": toJSStrings,
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(reportTemplate)
	if err != nil {
		log.Fatalf("template parse: %v", err)
	}

	out, err := os.Create(*outFile)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, td); err != nil {
		log.Fatalf("template execute: %v", err)
	}

	fmt.Printf("Report written → %s\n", *outFile)
}
