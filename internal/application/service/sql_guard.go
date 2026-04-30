package service

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

var (
	ErrStructuredQuerySQLRequired          = errors.New("sql is required")
	ErrStructuredQueryLimitRequired        = errors.New("query limit is required")
	ErrStructuredQueryLimitExceeded        = errors.New("query limit exceeds the configured max rows")
	ErrStructuredQueryTableNotAllowed      = errors.New("query references a table outside the allowed schema")
	ErrStructuredQueryColumnNotAllowed     = errors.New("query references a column outside the allowed schema")
	ErrStructuredQuerySensitiveColumn      = errors.New("query references a sensitive or denied column")
	ErrStructuredQueryUserRequired         = errors.New("user id is required for query execution")
	ErrStructuredQueryKnowledgeBaseMissing = errors.New("knowledge base id is required")
)

var wildcardSelectPattern = regexp.MustCompile(`(?is)(^|,)\s*(?:[a-zA-Z_][\w$]*\s*\.)?\*\s*(,|from\b)`)

type sqlGuard struct{}

type sqlGuardPolicy struct {
	dialect          types.SQLDialect
	allowedTables    map[string]struct{}
	visibleColumns   map[string]map[string]struct{}
	sensitiveColumns map[string]map[string]struct{}
	deniedColumns    map[string]map[string]struct{}
	maxRows          int
	timeout          time.Duration
}

func newSQLGuard() *sqlGuard {
	return &sqlGuard{}
}

func (g *sqlGuard) Validate(
	sqlText string,
	schema *types.DatabaseSchema,
	cfg *types.DatabaseConnectionConfig,
	dialect types.SQLDialect,
	requestedMaxRows int,
	requestedTimeoutSeconds int,
) (*types.ValidatedSQL, error) {
	trimmedSQL := strings.TrimSpace(sqlText)
	trimmedSQL = strings.TrimSuffix(trimmedSQL, ";")
	trimmedSQL = strings.TrimSpace(trimmedSQL)
	if trimmedSQL == "" {
		return nil, ErrStructuredQuerySQLRequired
	}

	policy := buildSQLGuardPolicy(schema, cfg, dialect, requestedMaxRows, requestedTimeoutSeconds)
	normalizedSQL := normalizeSQLForValidation(trimmedSQL, policy.dialect)
	cteNames := extractCTENames(normalizedSQL)
	cteTables := extractCTEBaseTables(normalizedSQL)
	parseResult, validation := utils.ValidateSQL(
		normalizedSQL,
		utils.WithInputValidation(6, 4096),
		utils.WithSelectOnly(),
		utils.WithSingleStatement(),
		utils.WithNoDangerousFunctions(),
		utils.WithInjectionRiskCheck(),
	)
	if validation == nil || !validation.Valid {
		return nil, formatSQLValidationError(validation)
	}
	if parseResult == nil || !parseResult.IsSelect {
		return nil, fmt.Errorf("%w: only SELECT queries are allowed", ErrStructuredQuerySQLRequired)
	}
	if !hasExplicitLimit(normalizedSQL) {
		return nil, ErrStructuredQueryLimitRequired
	}
	if limitValue, ok := extractLimitValue(normalizedSQL); ok && limitValue > policy.maxRows {
		return nil, fmt.Errorf("%w: limit=%d max_rows=%d", ErrStructuredQueryLimitExceeded, limitValue, policy.maxRows)
	}

	cleanedTables := make([]string, 0, len(parseResult.TableNames))
	seenTables := make(map[string]struct{})
	for _, tableName := range parseResult.TableNames {
		cleaned := normalizeIdentifier(tableName)
		if cleaned == "" {
			continue
		}
		if _, isCTE := cteNames[cleaned]; isCTE {
			continue
		}
		if _, allowed := policy.allowedTables[cleaned]; !allowed {
			return nil, fmt.Errorf("%w: %s", ErrStructuredQueryTableNotAllowed, tableName)
		}
		if _, seen := seenTables[cleaned]; !seen {
			seenTables[cleaned] = struct{}{}
			cleanedTables = append(cleanedTables, cleaned)
		}
	}
	for _, tableName := range cteTables {
		if _, allowed := policy.allowedTables[tableName]; !allowed {
			return nil, fmt.Errorf("%w: %s", ErrStructuredQueryTableNotAllowed, tableName)
		}
		if _, seen := seenTables[tableName]; seen {
			continue
		}
		seenTables[tableName] = struct{}{}
		cleanedTables = append(cleanedTables, tableName)
	}
	if len(cleanedTables) == 0 {
		return nil, ErrStructuredQueryTableNotAllowed
	}

	selectedFields := normalizeIdentifiers(parseResult.SelectFields)
	referencedFields := uniqueNormalizedIdentifiers(parseResult.SelectFields, parseResult.WhereFields)
	if isWildcardSelect(normalizedSQL) {
		for _, tableName := range cleanedTables {
			if len(policy.deniedColumns[tableName]) > 0 || len(policy.sensitiveColumns[tableName]) > 0 {
				return nil, fmt.Errorf("%w: wildcard selects are not allowed when protected columns exist on %s", ErrStructuredQuerySensitiveColumn, tableName)
			}
		}
	} else {
		for _, field := range referencedFields {
			if field == "" {
				continue
			}
			matchedTables := matchingTablesForColumn(field, cleanedTables, policy.visibleColumns)
			if len(matchedTables) == 0 {
				return nil, fmt.Errorf("%w: %s", ErrStructuredQueryColumnNotAllowed, field)
			}
			for _, tableName := range matchedTables {
				if _, denied := policy.deniedColumns[tableName][field]; denied {
					return nil, fmt.Errorf("%w: %s.%s", ErrStructuredQuerySensitiveColumn, tableName, field)
				}
				if _, sensitive := policy.sensitiveColumns[tableName][field]; sensitive {
					return nil, fmt.Errorf("%w: %s.%s", ErrStructuredQuerySensitiveColumn, tableName, field)
				}
			}
		}
	}

	sort.Strings(cleanedTables)
	return &types.ValidatedSQL{
		OriginalSQL:   trimmedSQL,
		ExecutedSQL:   trimmedSQL,
		NormalizedSQL: normalizedSQL,
		Dialect:       policy.dialect,
		Tables:        cleanedTables,
		SelectFields:  selectedFields,
		MaxRows:       policy.maxRows,
		Timeout:       policy.timeout,
	}, nil
}

func buildSQLGuardPolicy(
	schema *types.DatabaseSchema,
	cfg *types.DatabaseConnectionConfig,
	dialect types.SQLDialect,
	requestedMaxRows int,
	requestedTimeoutSeconds int,
) sqlGuardPolicy {
	policy := sqlGuardPolicy{
		dialect:          dialect,
		allowedTables:    make(map[string]struct{}),
		visibleColumns:   make(map[string]map[string]struct{}),
		sensitiveColumns: make(map[string]map[string]struct{}),
		deniedColumns:    make(map[string]map[string]struct{}),
		maxRows:          clampPositiveInt(requestedMaxRows, databaseConfigMaxRows(cfg), 500),
		timeout:          time.Duration(clampPositiveInt(requestedTimeoutSeconds, databaseConfigTimeoutSeconds(cfg), 10)) * time.Second,
	}
	if schema != nil {
		for _, table := range schema.Tables {
			tableName := normalizeIdentifier(table.Name)
			if tableName == "" {
				continue
			}
			policy.allowedTables[tableName] = struct{}{}
			if _, ok := policy.visibleColumns[tableName]; !ok {
				policy.visibleColumns[tableName] = make(map[string]struct{})
			}
			if _, ok := policy.sensitiveColumns[tableName]; !ok {
				policy.sensitiveColumns[tableName] = make(map[string]struct{})
			}
			for _, column := range table.Columns {
				columnName := normalizeIdentifier(column.Name)
				if columnName == "" {
					continue
				}
				policy.visibleColumns[tableName][columnName] = struct{}{}
				if column.IsSensitive {
					policy.sensitiveColumns[tableName][columnName] = struct{}{}
				}
			}
		}
	}
	if cfg != nil {
		for _, entry := range cfg.Settings.ColumnDenylist {
			tableName, columnName := parseColumnReference(entry)
			if tableName == "" || columnName == "" {
				continue
			}
			if _, ok := policy.deniedColumns[tableName]; !ok {
				policy.deniedColumns[tableName] = make(map[string]struct{})
			}
			policy.deniedColumns[tableName][columnName] = struct{}{}
		}
	}
	return policy
}

func normalizeSQLForValidation(sqlText string, dialect types.SQLDialect) string {
	normalized := strings.TrimSpace(sqlText)
	if dialect == types.SQLDialectMySQL {
		normalized = strings.ReplaceAll(normalized, "`", `"`)
	}
	return normalized
}

func formatSQLValidationError(validation *utils.SQLValidationResult) error {
	if validation == nil || len(validation.Errors) == 0 {
		return errors.New("sql validation failed")
	}
	parts := make([]string, 0, len(validation.Errors))
	for _, item := range validation.Errors {
		if item.Message == "" {
			continue
		}
		parts = append(parts, item.Message)
	}
	if len(parts) == 0 {
		return errors.New("sql validation failed")
	}
	return errors.New(strings.Join(parts, "; "))
}

func hasExplicitLimit(sqlText string) bool {
	return regexp.MustCompile(`(?is)\blimit\s+\d+`).MatchString(sqlText)
}

func extractLimitValue(sqlText string) (int, bool) {
	matches := regexp.MustCompile(`(?is)\blimit\s+(\d+)\s*(?:offset\s+\d+)?\s*$`).FindStringSubmatch(sqlText)
	if len(matches) == 2 {
		return parsePositiveInt(matches[1]), true
	}
	matches = regexp.MustCompile(`(?is)\boffset\s+\d+\s+limit\s+(\d+)\s*$`).FindStringSubmatch(sqlText)
	if len(matches) == 2 {
		return parsePositiveInt(matches[1]), true
	}
	return 0, false
}

func isWildcardSelect(sqlText string) bool {
	selectLower := strings.ToLower(sqlText)
	selectIndex := strings.Index(selectLower, "select")
	fromIndex := strings.Index(selectLower, " from ")
	if selectIndex == -1 || fromIndex == -1 || fromIndex <= selectIndex+6 {
		return wildcardSelectPattern.MatchString(sqlText)
	}
	selectList := sqlText[selectIndex+6 : fromIndex]
	return wildcardSelectPattern.MatchString(selectList + " from")
}

func parseColumnReference(value string) (string, string) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return "", ""
	}
	return normalizeIdentifier(parts[len(parts)-2]), normalizeIdentifier(parts[len(parts)-1])
}

func normalizeIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, `"'`)
	trimmed = strings.Trim(trimmed, "`")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ".")
	trimmed = parts[len(parts)-1]
	trimmed = strings.Trim(trimmed, `"'`)
	trimmed = strings.Trim(trimmed, "`")
	return strings.ToLower(trimmed)
}

func normalizeIdentifiers(values []string) []string {
	items := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		normalized := normalizeIdentifier(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		items = append(items, normalized)
	}
	return items
}

func uniqueNormalizedIdentifiers(groups ...[]string) []string {
	items := make([]string, 0)
	seen := make(map[string]struct{})
	for _, values := range groups {
		for _, value := range values {
			normalized := normalizeIdentifier(value)
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			items = append(items, normalized)
		}
	}
	return items
}

func matchingTablesForColumn(field string, tables []string, visibleColumns map[string]map[string]struct{}) []string {
	matched := make([]string, 0, len(tables))
	for _, tableName := range tables {
		if _, ok := visibleColumns[tableName][field]; ok {
			matched = append(matched, tableName)
		}
	}
	return matched
}

func extractCTENames(sqlText string) map[string]struct{} {
	definitions := extractCTEDefinitions(sqlText)
	items := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition.name == "" {
			continue
		}
		items[definition.name] = struct{}{}
	}
	return items
}

func extractCTEBaseTables(sqlText string) []string {
	definitions := extractCTEDefinitions(sqlText)
	if len(definitions) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	tables := make([]string, 0)
	for _, definition := range definitions {
		for _, tableName := range collectBaseTablesFromSQL(definition.query) {
			if _, ok := seen[tableName]; ok {
				continue
			}
			seen[tableName] = struct{}{}
			tables = append(tables, tableName)
		}
	}
	return tables
}

type cteDefinition struct {
	name  string
	query string
}

func extractCTEDefinitions(sqlText string) []cteDefinition {
	trimmed := strings.TrimSpace(sqlText)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "with ") && !strings.HasPrefix(lower, "with\n") && !strings.HasPrefix(lower, "with\t") && !strings.HasPrefix(lower, "with\r") && !strings.HasPrefix(lower, "with(") && !strings.HasPrefix(lower, "withrecursive") && !strings.HasPrefix(lower, "with recursive") {
		return nil
	}
	index := len("with")
	index = skipSQLWhitespace(trimmed, index)
	if strings.HasPrefix(strings.ToLower(trimmed[index:]), "recursive") {
		index += len("recursive")
		index = skipSQLWhitespace(trimmed, index)
	}

	definitions := make([]cteDefinition, 0)
	for index < len(trimmed) {
		name, next, ok := readSQLIdentifier(trimmed, index)
		if !ok {
			return definitions
		}
		index = skipSQLWhitespace(trimmed, next)
		if index < len(trimmed) && trimmed[index] == '(' {
			matched, end := scanBalancedSQL(trimmed, index, '(', ')')
			if !matched {
				return definitions
			}
			index = skipSQLWhitespace(trimmed, end)
		}
		if !strings.HasPrefix(strings.ToLower(trimmed[index:]), "as") {
			return definitions
		}
		index += len("as")
		index = skipSQLWhitespace(trimmed, index)
		if index >= len(trimmed) || trimmed[index] != '(' {
			return definitions
		}
		matched, end := scanBalancedSQL(trimmed, index, '(', ')')
		if !matched {
			return definitions
		}
		definitions = append(definitions, cteDefinition{
			name:  normalizeIdentifier(name),
			query: strings.TrimSpace(trimmed[index+1 : end-1]),
		})
		index = skipSQLWhitespace(trimmed, end)
		if index >= len(trimmed) || trimmed[index] != ',' {
			break
		}
		index++
		index = skipSQLWhitespace(trimmed, index)
	}
	return definitions
}

func collectBaseTablesFromSQL(sqlText string) []string {
	seen := make(map[string]struct{})
	tables := make([]string, 0)
	parseResult := utils.ParseSQL(sqlText)
	if parseResult != nil {
		cteNames := extractCTENames(sqlText)
		for _, tableName := range parseResult.TableNames {
			normalized := normalizeIdentifier(tableName)
			if normalized == "" {
				continue
			}
			if _, isCTE := cteNames[normalized]; isCTE {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			tables = append(tables, normalized)
		}
	}
	for _, tableName := range extractCTEBaseTables(sqlText) {
		if _, ok := seen[tableName]; ok {
			continue
		}
		seen[tableName] = struct{}{}
		tables = append(tables, tableName)
	}
	return tables
}

func skipSQLWhitespace(value string, index int) int {
	for index < len(value) {
		switch value[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}

func readSQLIdentifier(value string, index int) (string, int, bool) {
	index = skipSQLWhitespace(value, index)
	if index >= len(value) {
		return "", index, false
	}
	if value[index] == '"' || value[index] == '`' {
		quote := value[index]
		start := index + 1
		index = start
		for index < len(value) && value[index] != quote {
			index++
		}
		if index >= len(value) {
			return "", index, false
		}
		return value[start:index], index + 1, true
	}
	start := index
	for index < len(value) {
		ch := value[index]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$' {
			index++
			continue
		}
		break
	}
	if start == index {
		return "", index, false
	}
	return value[start:index], index, true
}

func scanBalancedSQL(value string, start int, open byte, close byte) (bool, int) {
	if start >= len(value) || value[start] != open {
		return false, start
	}
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	for index := start; index < len(value); index++ {
		ch := value[index]
		if ch == '\'' && !inDoubleQuote {
			if index == 0 || value[index-1] != '\\' {
				inSingleQuote = !inSingleQuote
			}
			continue
		}
		if ch == '"' && !inSingleQuote {
			if index == 0 || value[index-1] != '\\' {
				inDoubleQuote = !inDoubleQuote
			}
			continue
		}
		if inSingleQuote || inDoubleQuote {
			continue
		}
		if ch == open {
			depth++
			continue
		}
		if ch == close {
			depth--
			if depth == 0 {
				return true, index + 1
			}
		}
	}
	return false, len(value)
}

func databaseConfigMaxRows(cfg *types.DatabaseConnectionConfig) int {
	if cfg == nil {
		return 0
	}
	return cfg.Settings.MaxRows
}

func databaseConfigTimeoutSeconds(cfg *types.DatabaseConnectionConfig) int {
	if cfg == nil {
		return 0
	}
	return cfg.Settings.QueryTimeoutSec
}

func clampPositiveInt(requested int, configured int, fallback int) int {
	base := coalescePositiveInt(configured, fallback)
	if requested > 0 && requested < base {
		return requested
	}
	return base
}

func parsePositiveInt(value string) int {
	parsed := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		parsed = parsed*10 + int(ch-'0')
	}
	return parsed
}

func coalescePositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
