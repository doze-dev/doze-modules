package postgres

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The extension catalog: every extension doze ships as a prebuilt bundle, by its
// CREATE EXTENSION name. A bundle is an engine in the binaries mirror keyed
// pgext-<name>-pg<major> (doze/docs/design/postgres-extensions.md, "The contract
// between the build and the module"); doze-binaries' versions.yaml `extensions:`
// builds what this table names, so the two must agree on names, majors and
// version lines. Contrib extensions ship inside every Postgres and are not here.

// CatalogEntry is one extension doze can fetch by name.
type CatalogEntry struct {
	// Name is the CREATE EXTENSION name and the bundle key's <name>.
	Name string `json:"name" yaml:"name"`
	// Aliases are friendlier names that mean the same extension (pgvector).
	Aliases []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	// Wave is the rollout wave from the design (1 simple, 2 heavy).
	Wave int `json:"wave" yaml:"wave"`
	// Description is one line: what it is for.
	Description string `json:"description" yaml:"description"`
	// License is the bundle's licence as shipped.
	License string `json:"license" yaml:"license"`
	// Line is the default version line ("0.8"): a key of the mirror index's
	// versions map, which resolves to the newest release on that line.
	Line string `json:"line" yaml:"line"`
	// Lines overrides Line for majors that have their own release line
	// (pgaudit 17.x is for Postgres 17 only).
	Lines map[int]string `json:"lines,omitempty" yaml:"lines,omitempty"`
	// Majors are the Postgres majors the extension is built for.
	Majors []int `json:"majors" yaml:"majors"`
	// Preload names libraries that must be in shared_preload_libraries.
	Preload []string `json:"preload,omitempty" yaml:"preload,omitempty"`
	// MayPreload names libraries that are optional to preload (a background
	// worker, session-wide hints); add them with settings.shared_preload_libraries.
	MayPreload []string `json:"may_preload,omitempty" yaml:"may_preload,omitempty"`
	// Settings are postgresql.conf settings the extension needs; "{db}" is the
	// instance's database.
	Settings map[string]string `json:"settings,omitempty" yaml:"settings,omitempty"`
	// Env is environment the server needs; "{root}" is the bundle's root.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	// Requires are extensions created before this one (catalog or contrib).
	Requires []string `json:"requires,omitempty" yaml:"requires,omitempty"`
	// Provides are further CREATE EXTENSION names inside the same bundle, each
	// with what it requires (postgis_raster needs postgis).
	Provides map[string][]string `json:"provides,omitempty" yaml:"provides,omitempty"`
	// Note is anything a reader should know (a licence caveat).
	Note string `json:"note,omitempty" yaml:"note,omitempty"`
	// Homepage is the upstream project.
	Homepage string `json:"homepage" yaml:"homepage"`
}

var pg14to18 = []int{14, 15, 16, 17, 18}

// catalog lists waves 1 and 2 of the design. Version lines must be keys in the
// mirror index doze-binaries publishes (two-part, major_parts: 2). Wave 1's
// match doze-binaries' versions.yaml as of 2026-11-07; wave 2's (and the h3,
// pgrouting and age keys) are not built yet and must be checked when they are.
var catalog = []CatalogEntry{
	// ── wave 1 ──
	{Name: "vector", Aliases: []string{"pgvector"}, Wave: 1, Description: "Vector similarity search (HNSW, IVFFlat) for embeddings.",
		License: "PostgreSQL", Line: "0.8", Majors: pg14to18, Homepage: "https://github.com/pgvector/pgvector"},
	{Name: "pg_cron", Wave: 1, Description: "Cron-style scheduled jobs run inside the database.",
		License: "PostgreSQL", Line: "1.6", Majors: pg14to18, Preload: []string{"pg_cron"},
		// Background workers, not libpq connections: the backend has no TCP.
		Settings: map[string]string{"cron.database_name": "{db}", "cron.use_background_workers": "on"},
		Homepage: "https://github.com/citusdata/pg_cron"},
	{Name: "pg_partman", Wave: 1, Description: "Time- and serial-based partition management.",
		License: "PostgreSQL", Line: "5.5", Majors: pg14to18, Preload: []string{"pg_partman_bgw"},
		Settings: map[string]string{"pg_partman_bgw.dbname": "{db}"},
		Homepage: "https://github.com/pgpartman/pg_partman"},
	{Name: "pgmq", Wave: 1, Description: "Lightweight message queues, like SQS, in tables.",
		License: "PostgreSQL", Line: "1.13", Majors: pg14to18, Homepage: "https://github.com/pgmq/pgmq"},
	{Name: "hypopg", Wave: 1, Description: "Hypothetical indexes: see what an index would do without building it.",
		License: "PostgreSQL", Line: "1.4", Majors: pg14to18, Homepage: "https://github.com/HypoPG/hypopg"},
	{Name: "pg_hint_plan", Wave: 1, Description: "Steer the planner with hints in SQL comments.",
		License: "BSD-3-Clause", Majors: pg14to18, Preload: []string{"pg_hint_plan"},
		Lines:    map[int]string{14: "1.4", 15: "1.5", 16: "1.6", 17: "1.7", 18: "1.8"},
		Homepage: "https://github.com/ossc-db/pg_hint_plan"},
	{Name: "pg_repack", Wave: 1, Description: "Rebuild bloated tables and indexes online (with the pg_repack CLI).",
		License: "BSD-3-Clause", Line: "1.5", Majors: pg14to18, Homepage: "https://github.com/reorg/pg_repack"},
	{Name: "pgaudit", Wave: 1, Description: "Session and object audit logging.",
		License: "PostgreSQL", Majors: pg14to18, Preload: []string{"pgaudit"},
		Lines:    map[int]string{14: "1.6", 15: "1.7", 16: "16.1", 17: "17.1", 18: "18.0"},
		Homepage: "https://github.com/pgaudit/pgaudit"},
	{Name: "pg_ivm", Wave: 1, Description: "Incrementally maintained materialized views.",
		License: "PostgreSQL", Line: "1.17", Majors: pg14to18, Preload: []string{"pg_ivm"},
		Homepage: "https://github.com/sraoss/pg_ivm"},
	{Name: "pg_jsonschema", Wave: 1, Description: "JSON Schema validation for json and jsonb.",
		License: "Apache-2.0", Line: "0.3", Majors: pg14to18, Homepage: "https://github.com/supabase/pg_jsonschema"},

	// ── wave 2 ──
	{Name: "postgis", Wave: 2, Description: "Geographic objects, spatial indexes and functions.",
		License: "GPL-2.0-or-later", Line: "3.6", Majors: pg14to18,
		Provides: map[string][]string{
			"postgis_raster":               {"postgis"},
			"postgis_topology":             {"postgis"},
			"postgis_tiger_geocoder":       {"postgis", "fuzzystrmatch"},
			"address_standardizer":         nil,
			"address_standardizer_data_us": {"address_standardizer"},
		},
		Env:      map[string]string{"PROJ_DATA": "{root}/share/proj", "GDAL_DATA": "{root}/share/gdal"},
		Homepage: "https://postgis.net"},
	{Name: "h3", Wave: 2, Description: "Uber's H3 hexagonal grid indexing (h3_postgis bridges to PostGIS).",
		License: "Apache-2.0", Line: "4.2", Majors: pg14to18,
		Provides: map[string][]string{"h3_postgis": {"h3", "postgis"}},
		Homepage: "https://github.com/zachasme/h3-pg"},
	{Name: "pgrouting", Wave: 2, Description: "Shortest-path and network routing on PostGIS.",
		License: "GPL-2.0-or-later", Line: "3.8", Majors: pg14to18, Requires: []string{"postgis"},
		Homepage: "https://pgrouting.org"},
	{Name: "timescaledb", Aliases: []string{"timescale"}, Wave: 2, Description: "Time-series hypertables and time_bucket.",
		License: "Apache-2.0", Line: "2.31", Majors: []int{15, 16, 17, 18}, Preload: []string{"timescaledb"},
		Settings: map[string]string{"timescaledb.telemetry_level": "off", "timescaledb.license": "apache"},
		Note:     "The Apache-only build: hypertables and time_bucket; no compression, continuous aggregates or retention policies (those are under the Timescale License).",
		Homepage: "https://github.com/timescale/timescaledb"},
	{Name: "pg_search", Wave: 2, Description: "BM25 full-text search (ParadeDB).",
		License: "AGPL-3.0", Line: "0.20", Majors: []int{15, 16, 17, 18}, Preload: []string{"pg_search"},
		Homepage: "https://github.com/paradedb/paradedb"},
	{Name: "pg_duckdb", Wave: 2, Description: "DuckDB inside Postgres: fast analytics, Parquet and Iceberg.",
		License: "MIT", Line: "1.1", Majors: pg14to18, Preload: []string{"pg_duckdb"},
		Homepage: "https://github.com/duckdb/pg_duckdb"},
	{Name: "vectorscale", Aliases: []string{"pgvectorscale"}, Wave: 2, Description: "DiskANN indexes for pgvector at scale.",
		License: "PostgreSQL", Line: "0.9", Majors: []int{18}, Requires: []string{"vector"},
		Homepage: "https://github.com/timescale/pgvectorscale"},
	{Name: "age", Aliases: []string{"apache_age"}, Wave: 2, Description: "Graph database with openCypher queries (Apache AGE).",
		License: "Apache-2.0", Majors: pg14to18, MayPreload: []string{"age"},
		Lines:    map[int]string{14: "1.5", 15: "1.5", 16: "1.5", 17: "1.6", 18: "1.6"},
		Homepage: "https://age.apache.org"},
}

// contrib names ship with every Postgres doze builds, by the majors they exist
// in. CREATE EXTENSION finds them in the toolchain; there is nothing to fetch.
var contribAll = []string{
	"amcheck", "autoinc", "bloom", "btree_gin", "btree_gist", "citext", "cube", "dblink",
	"dict_int", "dict_xsyn", "earthdistance", "file_fdw", "fuzzystrmatch", "hstore",
	"insert_username", "intagg", "intarray", "isn", "lo", "ltree", "moddatetime",
	"pageinspect", "pg_buffercache", "pg_freespacemap", "pg_prewarm", "pg_stat_statements",
	"pg_surgery", "pg_trgm", "pg_visibility", "pgcrypto", "pgrowlocks", "pgstattuple",
	"plpgsql", "postgres_fdw", "refint", "seg", "sslinfo", "tablefunc", "tcn",
	"tsm_system_rows", "tsm_system_time", "unaccent", "uuid-ossp", "xml2",
}

// contribRange bounds the contrib names that come and go: [since, until].
var contribRange = map[string][2]int{
	"adminpack":         {0, 16},
	"old_snapshot":      {0, 16},
	"pg_walinspect":     {15, 0},
	"pg_logicalinspect": {18, 0},
}

// isContrib reports whether name ships with Postgres major (0 = unknown major,
// any contrib name counts).
func isContrib(name string, major int) bool {
	for _, c := range contribAll {
		if c == name {
			return true
		}
	}
	r, ok := contribRange[name]
	if !ok {
		return false
	}
	if major == 0 {
		return true
	}
	return (r[0] == 0 || major >= r[0]) && (r[1] == 0 || major <= r[1])
}

// catalogRef is a declared name resolved against the catalog: the entry whose
// bundle holds it, and the CREATE EXTENSION name (an entry's own or one it
// provides).
type catalogRef struct {
	Entry    *CatalogEntry
	Create   string   // CREATE EXTENSION name
	Requires []string // created first
}

// lookupCatalog resolves a name or alias, or a name a bundle provides.
func lookupCatalog(name string) (catalogRef, bool) {
	for i := range catalog {
		e := &catalog[i]
		if e.Name == name || contains(e.Aliases, name) {
			return catalogRef{Entry: e, Create: e.Name, Requires: e.Requires}, true
		}
		if req, ok := e.Provides[name]; ok {
			return catalogRef{Entry: e, Create: name, Requires: req}, true
		}
	}
	return catalogRef{}, false
}

// canonicalName maps an alias to the CREATE EXTENSION name; anything else
// passes through.
func canonicalName(name string) string {
	if ref, ok := lookupCatalog(name); ok {
		return ref.Create
	}
	return name
}

// supports reports whether the entry is built for the Postgres major.
func (e *CatalogEntry) supports(major int) bool {
	for _, m := range e.Majors {
		if m == major {
			return true
		}
	}
	return false
}

// line returns the default version line for a major.
func (e *CatalogEntry) line(major int) string {
	if l, ok := e.Lines[major]; ok {
		return l
	}
	return e.Line
}

// engineKey is the mirror engine a bundle is published under.
func engineKey(name string, major int) string { return fmt.Sprintf("pgext-%s-pg%d", name, major) }

// checkExtension validates a declared extension name for a Postgres major
// (0 = not known yet): contrib, a catalog name supported on that major, or an
// error that names the closest match and the catalog.
func checkExtension(name string, major int) error {
	if isContrib(name, major) {
		return nil
	}
	ref, ok := lookupCatalog(name)
	if !ok {
		return unknownExtensionError(name, major)
	}
	if major != 0 && !ref.Entry.supports(major) {
		return fmt.Errorf("%s needs Postgres %s; this block declares %d", ref.Entry.Name, majorsPhrase(ref.Entry.Majors), major)
	}
	return nil
}

func unknownExtensionError(name string, major int) error {
	var b strings.Builder
	fmt.Fprintf(&b, "doze has no extension %s", name)
	if major != 0 {
		fmt.Fprintf(&b, " for Postgres %d", major)
	}
	if near := didYouMean(name, major); near != "" {
		fmt.Fprintf(&b, "; did you mean %s?", near)
	} else {
		b.WriteString(";")
	}
	fmt.Fprintf(&b, " The catalog: %s; contrib extensions (pg_trgm, hstore, …) work too; or give source = a bundle URL", strings.Join(catalogNames(major), ", "))
	return fmt.Errorf("%s", b.String())
}

// catalogNames lists the catalog's names available on a major (all when 0).
func catalogNames(major int) []string {
	var out []string
	for i := range catalog {
		if major == 0 || catalog[i].supports(major) {
			out = append(out, catalog[i].Name)
		}
	}
	return out
}

// didYouMean returns the catalog or contrib name nearest to name, if close.
func didYouMean(name string, major int) string {
	cands := append([]string{}, contribAll...)
	for i := range catalog {
		e := &catalog[i]
		if major != 0 && !e.supports(major) {
			continue
		}
		cands = append(cands, e.Name)
		cands = append(cands, e.Aliases...)
		for p := range e.Provides {
			cands = append(cands, p)
		}
	}
	sort.Strings(cands)
	best, bestD := "", 1<<30
	for _, c := range cands {
		if d := editDistance(strings.ToLower(name), c); d < bestD {
			best, bestD = c, d
		}
	}
	// Close: at most a third of the name wrong (vectr → vector, pgcron → pg_cron).
	if bestD <= max(2, len(name)/3) {
		return canonicalName(best)
	}
	return ""
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// majorsPhrase renders a support list: "15 or newer", "18", "14–16".
func majorsPhrase(ms []int) string {
	if len(ms) == 0 {
		return "(no major)"
	}
	lo, hi := ms[0], ms[len(ms)-1]
	contiguous := hi-lo+1 == len(ms)
	switch {
	case len(ms) == 1:
		return strconv.Itoa(lo)
	case contiguous && hi == latestMajor:
		return fmt.Sprintf("%d or newer", lo)
	case contiguous:
		return fmt.Sprintf("%d–%d", lo, hi)
	}
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = strconv.Itoa(m)
	}
	return strings.Join(parts, ", ")
}

// latestMajor is the newest Postgres the module runs (Describe's Versions).
const latestMajor = 18

// checkExtensionVersion validates a catalog pin: a line ("0.8") or an exact
// release ("0.8.1"). Contrib versions are SQL versions and pass through.
func checkExtensionVersion(name, version string) error {
	if version == "" {
		return nil
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("extension %s: version %q should be a line like 0.8 or a release like 0.8.1", name, version)
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return fmt.Errorf("extension %s: version %q should be a line like 0.8 or a release like 0.8.1", name, version)
		}
	}
	return nil
}

// Catalog returns the extension catalog for the registry page (a copy).
func Catalog() []CatalogEntry {
	out := make([]CatalogEntry, len(catalog))
	copy(out, catalog)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// majorNum parses a Postgres major from a version spec ("17", "17.4"); 0 when
// the spec is empty or not numeric.
func majorNum(spec string) int {
	m, _, _ := strings.Cut(spec, ".")
	n, err := strconv.Atoi(m)
	if err != nil {
		return 0
	}
	return n
}
