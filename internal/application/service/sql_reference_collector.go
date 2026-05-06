package service

import (
	"fmt"
	"sort"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

type sqlColumnReference struct {
	Table      string
	TableAlias string
	Column     string
	Clause     string
	Derived    bool
}

type sqlTableReference struct {
	Table string
	Alias string
}

type sqlReferenceSet struct {
	Tables              []sqlTableReference
	Columns             []sqlColumnReference
	Wildcards           []sqlColumnReference
	CTEs                map[string]struct{}
	SelectFields        []string
	HasWildcard         bool
	HasDistinct         bool
	HasGroupBy          bool
	HasWindow           bool
	HasAggregate        bool
	HasExplicitLimit    bool
	PureGlobalAggregate bool
}

type sqlCollectorScope struct {
	parent       *sqlCollectorScope
	relations    map[string]*sqlRelationBinding
	relationList []*sqlRelationBinding
	cteColumns   map[string]*sqlDerivedRelation
}

type sqlRelationBinding struct {
	name       string
	baseTable  string
	columns    map[string]struct{}
	columnList []string
	derived    bool
}

type sqlDerivedRelation struct {
	columns    map[string]struct{}
	columnList []string
}

func collectSQLReferences(sqlText string, policy sqlGuardPolicy) (*sqlReferenceSet, error) {
	parsed, err := pg_query.Parse(sqlText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SQL references: %w", err)
	}
	if len(parsed.Stmts) == 0 || parsed.Stmts[0].Stmt == nil {
		return nil, fmt.Errorf("failed to parse SQL references: empty statement")
	}
	selectStmt := parsed.Stmts[0].Stmt.GetSelectStmt()
	if selectStmt == nil {
		return nil, fmt.Errorf("failed to parse SQL references: only SELECT statements are supported")
	}

	references, _, err := collectSelectReferences(selectStmt, newSQLCollectorScope(nil), policy)
	if err != nil {
		return nil, err
	}
	references.HasExplicitLimit = selectStmt.GetLimitCount() != nil
	references.SelectFields = uniqueNormalizedStrings(references.SelectFields)
	return references, nil
}

func newSQLCollectorScope(parent *sqlCollectorScope) *sqlCollectorScope {
	return &sqlCollectorScope{
		parent:       parent,
		relations:    make(map[string]*sqlRelationBinding),
		relationList: make([]*sqlRelationBinding, 0),
		cteColumns:   make(map[string]*sqlDerivedRelation),
	}
}

func (s *sqlCollectorScope) addRelation(binding *sqlRelationBinding) {
	if s == nil || binding == nil || binding.name == "" {
		return
	}
	s.relations[binding.name] = binding
	s.relationList = append(s.relationList, binding)
}

func (s *sqlCollectorScope) addCTE(name string, relation *sqlDerivedRelation) {
	if s == nil || name == "" || relation == nil {
		return
	}
	s.cteColumns[name] = relation
}

func (s *sqlCollectorScope) lookupRelation(name string) *sqlRelationBinding {
	if s == nil || name == "" {
		return nil
	}
	if binding, ok := s.relations[name]; ok {
		return binding
	}
	if s.parent != nil {
		return s.parent.lookupRelation(name)
	}
	return nil
}

func (s *sqlCollectorScope) lookupCTE(name string) *sqlDerivedRelation {
	if s == nil || name == "" {
		return nil
	}
	if relation, ok := s.cteColumns[name]; ok {
		return relation
	}
	if s.parent != nil {
		return s.parent.lookupCTE(name)
	}
	return nil
}

func (s *sqlCollectorScope) findColumn(name string) []*sqlRelationBinding {
	if s == nil || name == "" {
		return nil
	}
	localMatches := make([]*sqlRelationBinding, 0)
	for _, binding := range s.relationList {
		if binding == nil {
			continue
		}
		if _, ok := binding.columns[name]; ok {
			localMatches = append(localMatches, binding)
		}
	}
	if len(localMatches) > 0 {
		return localMatches
	}
	if s.parent != nil {
		return s.parent.findColumn(name)
	}
	return nil
}

func (s *sqlCollectorScope) localBindings() []*sqlRelationBinding {
	if s == nil {
		return nil
	}
	return append([]*sqlRelationBinding(nil), s.relationList...)
}

func collectSelectReferences(selectStmt *pg_query.SelectStmt, parentScope *sqlCollectorScope, policy sqlGuardPolicy) (*sqlReferenceSet, []string, error) {
	references := &sqlReferenceSet{CTEs: make(map[string]struct{})}
	queryScope := newSQLCollectorScope(parentScope)

	if withClause := selectStmt.GetWithClause(); withClause != nil {
		for _, cteNode := range withClause.GetCtes() {
			cte := cteNode.GetCommonTableExpr()
			if cte == nil {
				continue
			}
			cteName := normalizeIdentifier(cte.GetCtename())
			if cteName == "" {
				continue
			}
			childRefs, childOutputs, err := collectQueryNodeReferences(cte.GetCtequery(), queryScope, policy)
			if err != nil {
				return nil, nil, err
			}
			mergeSQLReferenceSets(references, childRefs)
			outputColumns := childOutputs
			if aliasColumns := extractAliasColumnNames(cte.GetAliascolnames()); len(aliasColumns) > 0 {
				outputColumns = aliasColumns
			}
			queryScope.addCTE(cteName, &sqlDerivedRelation{
				columns:    stringSliceToSet(outputColumns),
				columnList: outputColumns,
			})
			references.CTEs[cteName] = struct{}{}
		}
	}

	bodyScope := newSQLCollectorScope(queryScope)
	for _, fromItem := range selectStmt.GetFromClause() {
		itemRefs, bindings, err := collectFromItemReferences(fromItem, bodyScope, policy)
		if err != nil {
			return nil, nil, err
		}
		mergeSQLReferenceSets(references, itemRefs)
		for _, binding := range bindings {
			bodyScope.addRelation(binding)
		}
	}

	if selectStmt.GetOp() != pg_query.SetOperation_SET_OPERATION_UNDEFINED {
		if selectStmt.GetLarg() != nil {
			leftRefs, leftOutputs, err := collectSelectReferences(selectStmt.GetLarg(), queryScope, policy)
			if err != nil {
				return nil, nil, err
			}
			mergeSQLReferenceSets(references, leftRefs)
			if selectStmt.GetRarg() != nil {
				rightRefs, _, err := collectSelectReferences(selectStmt.GetRarg(), queryScope, policy)
				if err != nil {
					return nil, nil, err
				}
				mergeSQLReferenceSets(references, rightRefs)
			}
			references.HasDistinct = references.HasDistinct || len(selectStmt.GetDistinctClause()) > 0
			references.HasGroupBy = references.HasGroupBy || len(selectStmt.GetGroupClause()) > 0
			return references, leftOutputs, nil
		}
	}

	aggregateOnlyTargets := len(selectStmt.GetTargetList()) > 0
	outputColumns := make([]string, 0, len(selectStmt.GetTargetList()))
	for index, targetNode := range selectStmt.GetTargetList() {
		resTarget := targetNode.GetResTarget()
		if resTarget == nil {
			aggregateOnlyTargets = false
			continue
		}
		aggregateOnly, err := collectExpressionReferences(resTarget.GetVal(), "select", bodyScope, policy, references, false)
		if err != nil {
			return nil, nil, err
		}
		if !aggregateOnly {
			aggregateOnlyTargets = false
		}
		outputColumns = append(outputColumns, deriveResultColumnName(resTarget, bodyScope, index)...)
	}

	if len(selectStmt.GetDistinctClause()) > 0 {
		references.HasDistinct = true
		for _, clause := range selectStmt.GetDistinctClause() {
			if clause == nil {
				continue
			}
			if _, err := collectExpressionReferences(clause, "distinct", bodyScope, policy, references, false); err != nil {
				return nil, nil, err
			}
		}
	}

	if whereClause := selectStmt.GetWhereClause(); whereClause != nil {
		if _, err := collectExpressionReferences(whereClause, "where", bodyScope, policy, references, false); err != nil {
			return nil, nil, err
		}
	}

	if len(selectStmt.GetGroupClause()) > 0 {
		references.HasGroupBy = true
		for _, clause := range selectStmt.GetGroupClause() {
			if _, err := collectExpressionReferences(clause, "group_by", bodyScope, policy, references, false); err != nil {
				return nil, nil, err
			}
		}
	}

	if havingClause := selectStmt.GetHavingClause(); havingClause != nil {
		if _, err := collectExpressionReferences(havingClause, "having", bodyScope, policy, references, false); err != nil {
			return nil, nil, err
		}
	}

	for _, clause := range selectStmt.GetSortClause() {
		if _, err := collectExpressionReferences(clause, "order_by", bodyScope, policy, references, false); err != nil {
			return nil, nil, err
		}
	}

	for _, clause := range selectStmt.GetWindowClause() {
		references.HasWindow = true
		if _, err := collectExpressionReferences(clause, "window", bodyScope, policy, references, false); err != nil {
			return nil, nil, err
		}
	}

	references.PureGlobalAggregate = references.HasAggregate && !references.HasGroupBy && !references.HasWindow && aggregateOnlyTargets
	return references, uniqueNormalizedStrings(outputColumns), nil
}

func collectQueryNodeReferences(node *pg_query.Node, scope *sqlCollectorScope, policy sqlGuardPolicy) (*sqlReferenceSet, []string, error) {
	if node == nil {
		return &sqlReferenceSet{CTEs: make(map[string]struct{})}, nil, nil
	}
	if selectStmt := node.GetSelectStmt(); selectStmt != nil {
		return collectSelectReferences(selectStmt, scope, policy)
	}
	return &sqlReferenceSet{CTEs: make(map[string]struct{})}, nil, nil
}

func collectFromItemReferences(node *pg_query.Node, scope *sqlCollectorScope, policy sqlGuardPolicy) (*sqlReferenceSet, []*sqlRelationBinding, error) {
	references := &sqlReferenceSet{CTEs: make(map[string]struct{})}
	if node == nil {
		return references, nil, nil
	}

	if rangeVar := node.GetRangeVar(); rangeVar != nil {
		tableName := normalizeIdentifier(rangeVar.GetRelname())
		if tableName == "" {
			return references, nil, nil
		}
		aliasName := normalizeIdentifier(rangeVar.GetAlias().GetAliasname())
		bindingName := tableName
		if aliasName != "" {
			bindingName = aliasName
		}
		if cteRelation := scope.lookupCTE(tableName); cteRelation != nil {
			return references, []*sqlRelationBinding{{
				name:       bindingName,
				columns:    copyStringSet(cteRelation.columns),
				columnList: append([]string(nil), cteRelation.columnList...),
				derived:    true,
			}}, nil
		}
		references.Tables = append(references.Tables, sqlTableReference{Table: tableName, Alias: bindingName})
		return references, []*sqlRelationBinding{{
			name:       bindingName,
			baseTable:  tableName,
			columns:    copyStringSet(policy.visibleColumns[tableName]),
			columnList: sortedSetKeys(policy.visibleColumns[tableName]),
		}}, nil
	}

	if joinExpr := node.GetJoinExpr(); joinExpr != nil {
		leftRefs, leftBindings, err := collectFromItemReferences(joinExpr.GetLarg(), scope, policy)
		if err != nil {
			return nil, nil, err
		}
		mergeSQLReferenceSets(references, leftRefs)
		rightRefs, rightBindings, err := collectFromItemReferences(joinExpr.GetRarg(), scope, policy)
		if err != nil {
			return nil, nil, err
		}
		mergeSQLReferenceSets(references, rightRefs)

		joinScope := newSQLCollectorScope(scope)
		for _, binding := range leftBindings {
			joinScope.addRelation(binding)
		}
		for _, binding := range rightBindings {
			joinScope.addRelation(binding)
		}

		if quals := joinExpr.GetQuals(); quals != nil {
			if _, err := collectExpressionReferences(quals, "join_on", joinScope, policy, references, false); err != nil {
				return nil, nil, err
			}
		}
		for _, usingNode := range joinExpr.GetUsingClause() {
			if _, err := collectExpressionReferences(usingNode, "join_on", joinScope, policy, references, false); err != nil {
				return nil, nil, err
			}
		}

		if alias := normalizeIdentifier(joinExpr.GetAlias().GetAliasname()); alias != "" {
			return references, []*sqlRelationBinding{{
				name:       alias,
				columns:    stringSliceToSet(unionColumnLists(leftBindings, rightBindings)),
				columnList: unionColumnLists(leftBindings, rightBindings),
				derived:    true,
			}}, nil
		}
		return references, append(leftBindings, rightBindings...), nil
	}

	if subselect := node.GetRangeSubselect(); subselect != nil {
		childRefs, childOutputs, err := collectQueryNodeReferences(subselect.GetSubquery(), scope, policy)
		if err != nil {
			return nil, nil, err
		}
		mergeSQLReferenceSets(references, childRefs)
		aliasName := normalizeIdentifier(subselect.GetAlias().GetAliasname())
		if aliasName == "" {
			return references, nil, nil
		}
		return references, []*sqlRelationBinding{{
			name:       aliasName,
			columns:    stringSliceToSet(childOutputs),
			columnList: childOutputs,
			derived:    true,
		}}, nil
	}

	return references, nil, nil
}

func collectExpressionReferences(node *pg_query.Node, clause string, scope *sqlCollectorScope, policy sqlGuardPolicy, references *sqlReferenceSet, insideAggregate bool) (bool, error) {
	if node == nil {
		return true, nil
	}

	if resTarget := node.GetResTarget(); resTarget != nil {
		return collectExpressionReferences(resTarget.GetVal(), clause, scope, policy, references, insideAggregate)
	}
	if sortBy := node.GetSortBy(); sortBy != nil {
		return collectExpressionReferences(sortBy.GetNode(), clause, scope, policy, references, insideAggregate)
	}
	if windowDef := node.GetWindowDef(); windowDef != nil {
		references.HasWindow = true
		aggregateOnly := false
		for _, item := range windowDef.GetPartitionClause() {
			if _, err := collectExpressionReferences(item, clause, scope, policy, references, false); err != nil {
				return false, err
			}
		}
		for _, item := range windowDef.GetOrderClause() {
			if _, err := collectExpressionReferences(item, clause, scope, policy, references, false); err != nil {
				return false, err
			}
		}
		if _, err := collectExpressionReferences(windowDef.GetStartOffset(), clause, scope, policy, references, false); err != nil {
			return false, err
		}
		if _, err := collectExpressionReferences(windowDef.GetEndOffset(), clause, scope, policy, references, false); err != nil {
			return false, err
		}
		return aggregateOnly, nil
	}

	if colRef := node.GetColumnRef(); colRef != nil {
		return collectColumnReference(colRef, clause, scope, references, insideAggregate)
	}
	if aExpr := node.GetAExpr(); aExpr != nil {
		left, err := collectExpressionReferences(aExpr.GetLexpr(), clause, scope, policy, references, insideAggregate)
		if err != nil {
			return false, err
		}
		right, err := collectExpressionReferences(aExpr.GetRexpr(), clause, scope, policy, references, insideAggregate)
		if err != nil {
			return false, err
		}
		return left && right, nil
	}
	if boolExpr := node.GetBoolExpr(); boolExpr != nil {
		aggregateOnly := true
		for _, arg := range boolExpr.GetArgs() {
			childAggregate, err := collectExpressionReferences(arg, clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		return aggregateOnly, nil
	}
	if funcCall := node.GetFuncCall(); funcCall != nil {
		functionName := normalizeFunctionName(funcCall.GetFuncname())
		isWindow := funcCall.GetOver() != nil
		isAggregate := !isWindow && isAggregateFunction(functionName)
		if isWindow {
			references.HasWindow = true
		}
		if isAggregate {
			references.HasAggregate = true
		}
		aggregateOnly := !isWindow
		for _, arg := range funcCall.GetArgs() {
			childAggregate, err := collectExpressionReferences(arg, clause, scope, policy, references, insideAggregate || isAggregate)
			if err != nil {
				return false, err
			}
			if !isAggregate {
				aggregateOnly = aggregateOnly && childAggregate
			}
		}
		for _, item := range funcCall.GetAggOrder() {
			childAggregate, err := collectExpressionReferences(item, clause, scope, policy, references, insideAggregate || isAggregate)
			if err != nil {
				return false, err
			}
			if !isAggregate {
				aggregateOnly = aggregateOnly && childAggregate
			}
		}
		if _, err := collectExpressionReferences(funcCall.GetAggFilter(), clause, scope, policy, references, insideAggregate || isAggregate); err != nil {
			return false, err
		}
		if _, err := collectExpressionReferences(nodeFromWindowDef(funcCall.GetOver()), clause, scope, policy, references, false); err != nil {
			return false, err
		}
		if isAggregate {
			return true, nil
		}
		return aggregateOnly, nil
	}
	if typeCast := node.GetTypeCast(); typeCast != nil {
		return collectExpressionReferences(typeCast.GetArg(), clause, scope, policy, references, insideAggregate)
	}
	if collate := node.GetCollateClause(); collate != nil {
		return collectExpressionReferences(collate.GetArg(), clause, scope, policy, references, insideAggregate)
	}
	if indirection := node.GetAIndirection(); indirection != nil {
		aggregateOnly, err := collectExpressionReferences(indirection.GetArg(), clause, scope, policy, references, insideAggregate)
		if err != nil {
			return false, err
		}
		for _, item := range indirection.GetIndirection() {
			childAggregate, err := collectExpressionReferences(item, clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		return aggregateOnly, nil
	}
	if nullTest := node.GetNullTest(); nullTest != nil {
		return collectExpressionReferences(nullTest.GetArg(), clause, scope, policy, references, insideAggregate)
	}
	if booleanTest := node.GetBooleanTest(); booleanTest != nil {
		return collectExpressionReferences(booleanTest.GetArg(), clause, scope, policy, references, insideAggregate)
	}
	if caseExpr := node.GetCaseExpr(); caseExpr != nil {
		aggregateOnly := true
		if caseExpr.GetArg() != nil {
			childAggregate, err := collectExpressionReferences(caseExpr.GetArg(), clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		for _, arg := range caseExpr.GetArgs() {
			childAggregate, err := collectExpressionReferences(arg, clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		if caseExpr.GetDefresult() != nil {
			childAggregate, err := collectExpressionReferences(caseExpr.GetDefresult(), clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		return aggregateOnly, nil
	}
	if caseWhen := node.GetCaseWhen(); caseWhen != nil {
		exprAggregate, err := collectExpressionReferences(caseWhen.GetExpr(), clause, scope, policy, references, insideAggregate)
		if err != nil {
			return false, err
		}
		resultAggregate, err := collectExpressionReferences(caseWhen.GetResult(), clause, scope, policy, references, insideAggregate)
		if err != nil {
			return false, err
		}
		return exprAggregate && resultAggregate, nil
	}
	if coalesceExpr := node.GetCoalesceExpr(); coalesceExpr != nil {
		aggregateOnly := true
		for _, arg := range coalesceExpr.GetArgs() {
			childAggregate, err := collectExpressionReferences(arg, clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		return aggregateOnly, nil
	}
	if minMaxExpr := node.GetMinMaxExpr(); minMaxExpr != nil {
		aggregateOnly := true
		for _, arg := range minMaxExpr.GetArgs() {
			childAggregate, err := collectExpressionReferences(arg, clause, scope, policy, references, insideAggregate)
			if err != nil {
				return false, err
			}
			aggregateOnly = aggregateOnly && childAggregate
		}
		return aggregateOnly, nil
	}
	if subLink := node.GetSubLink(); subLink != nil {
		if _, err := collectExpressionReferences(subLink.GetTestexpr(), clause, scope, policy, references, insideAggregate); err != nil {
			return false, err
		}
		childRefs, _, err := collectQueryNodeReferences(subLink.GetSubselect(), newSQLCollectorScope(scope), policy)
		if err != nil {
			return false, err
		}
		mergeSQLReferenceSets(references, childRefs)
		return false, nil
	}
	if node.GetAConst() != nil {
		return true, nil
	}

	return false, nil
}

func collectColumnReference(colRef *pg_query.ColumnRef, clause string, scope *sqlCollectorScope, references *sqlReferenceSet, insideAggregate bool) (bool, error) {
	parts := columnRefFields(colRef)
	if len(parts) == 0 {
		return insideAggregate, nil
	}
	if parts[len(parts)-1] == "*" {
		references.HasWildcard = true
		for _, wildcard := range resolveWildcardReferences(parts, clause, scope) {
			references.Wildcards = append(references.Wildcards, wildcard)
		}
		return insideAggregate, nil
	}
	reference, err := resolveColumnReference(parts, clause, scope)
	if err != nil {
		return false, err
	}
	if !reference.Derived {
		references.Columns = append(references.Columns, reference)
		if clause == "select" {
			references.SelectFields = append(references.SelectFields, reference.Column)
		}
		return insideAggregate, nil
	}
	return insideAggregate, nil
}

func resolveColumnReference(parts []string, clause string, scope *sqlCollectorScope) (sqlColumnReference, error) {
	columnName := normalizeIdentifier(parts[len(parts)-1])
	if columnName == "" {
		return sqlColumnReference{}, fmt.Errorf("%w: invalid column reference", ErrStructuredQueryColumnNotAllowed)
	}

	if len(parts) == 1 {
		matches := scope.findColumn(columnName)
		if len(matches) == 0 {
			return sqlColumnReference{}, fmt.Errorf("%w: %s", ErrStructuredQueryColumnNotAllowed, columnName)
		}
		if len(matches) > 1 {
			return sqlColumnReference{}, fmt.Errorf("%w: ambiguous column %s", ErrStructuredQueryColumnNotAllowed, columnName)
		}
		return buildColumnReference(matches[0], columnName, clause), nil
	}

	qualifier := normalizeIdentifier(parts[len(parts)-2])
	binding := scope.lookupRelation(qualifier)
	if binding == nil {
		return sqlColumnReference{}, fmt.Errorf("%w: %s.%s", ErrStructuredQueryColumnNotAllowed, qualifier, columnName)
	}
	if _, ok := binding.columns[columnName]; !ok {
		return sqlColumnReference{}, fmt.Errorf("%w: %s.%s", ErrStructuredQueryColumnNotAllowed, qualifier, columnName)
	}
	return buildColumnReference(binding, columnName, clause), nil
}

func buildColumnReference(binding *sqlRelationBinding, columnName string, clause string) sqlColumnReference {
	reference := sqlColumnReference{
		TableAlias: binding.name,
		Column:     columnName,
		Clause:     clause,
		Derived:    binding.derived || binding.baseTable == "",
	}
	if !reference.Derived {
		reference.Table = binding.baseTable
	}
	return reference
}

func resolveWildcardReferences(parts []string, clause string, scope *sqlCollectorScope) []sqlColumnReference {
	if scope == nil {
		return nil
	}
	if len(parts) == 1 {
		refs := make([]sqlColumnReference, 0)
		for _, binding := range scope.localBindings() {
			if binding == nil || binding.baseTable == "" {
				continue
			}
			refs = append(refs, sqlColumnReference{Table: binding.baseTable, TableAlias: binding.name, Column: "*", Clause: clause})
		}
		return refs
	}
	qualifier := normalizeIdentifier(parts[len(parts)-2])
	binding := scope.lookupRelation(qualifier)
	if binding == nil || binding.baseTable == "" {
		return nil
	}
	return []sqlColumnReference{{Table: binding.baseTable, TableAlias: binding.name, Column: "*", Clause: clause}}
}

func deriveResultColumnName(target *pg_query.ResTarget, scope *sqlCollectorScope, index int) []string {
	if target == nil {
		return nil
	}
	if alias := normalizeIdentifier(target.GetName()); alias != "" {
		return []string{alias}
	}
	if target.GetVal() == nil {
		return nil
	}
	if colRef := target.GetVal().GetColumnRef(); colRef != nil {
		parts := columnRefFields(colRef)
		if len(parts) == 0 {
			return nil
		}
		if parts[len(parts)-1] == "*" {
			return expandWildcardOutputColumns(parts, scope)
		}
		name := normalizeIdentifier(parts[len(parts)-1])
		if name != "" {
			return []string{name}
		}
	}
	if funcCall := target.GetVal().GetFuncCall(); funcCall != nil {
		if name := normalizeFunctionName(funcCall.GetFuncname()); name != "" {
			return []string{name}
		}
	}
	if typeCast := target.GetVal().GetTypeCast(); typeCast != nil {
		return deriveResultColumnName(&pg_query.ResTarget{Val: typeCast.GetArg()}, scope, index)
	}
	if collate := target.GetVal().GetCollateClause(); collate != nil {
		return deriveResultColumnName(&pg_query.ResTarget{Val: collate.GetArg()}, scope, index)
	}
	if indirection := target.GetVal().GetAIndirection(); indirection != nil {
		return deriveResultColumnName(&pg_query.ResTarget{Val: indirection.GetArg()}, scope, index)
	}
	return nil
}

func expandWildcardOutputColumns(parts []string, scope *sqlCollectorScope) []string {
	if scope == nil {
		return nil
	}
	if len(parts) == 1 {
		outputs := make([]string, 0)
		seen := make(map[string]struct{})
		for _, binding := range scope.localBindings() {
			for _, columnName := range binding.columnList {
				if _, ok := seen[columnName]; ok {
					continue
				}
				seen[columnName] = struct{}{}
				outputs = append(outputs, columnName)
			}
		}
		return outputs
	}
	qualifier := normalizeIdentifier(parts[len(parts)-2])
	binding := scope.lookupRelation(qualifier)
	if binding == nil {
		return nil
	}
	return append([]string(nil), binding.columnList...)
}

func extractAliasColumnNames(nodes []*pg_query.Node) []string {
	items := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if strNode := node.GetString_(); strNode != nil {
			if normalized := normalizeIdentifier(strNode.GetSval()); normalized != "" {
				items = append(items, normalized)
			}
		}
	}
	return uniqueNormalizedStrings(items)
}

func columnRefFields(colRef *pg_query.ColumnRef) []string {
	items := make([]string, 0, len(colRef.GetFields()))
	for _, field := range colRef.GetFields() {
		if field == nil {
			continue
		}
		if strNode := field.GetString_(); strNode != nil {
			items = append(items, normalizeIdentifier(strNode.GetSval()))
			continue
		}
		if field.GetAStar() != nil {
			items = append(items, "*")
		}
	}
	return items
}

func normalizeFunctionName(nodes []*pg_query.Node) string {
	if len(nodes) == 0 {
		return ""
	}
	for index := len(nodes) - 1; index >= 0; index-- {
		if strNode := nodes[index].GetString_(); strNode != nil {
			return normalizeIdentifier(strNode.GetSval())
		}
	}
	return ""
}

func isAggregateFunction(name string) bool {
	switch normalizeIdentifier(name) {
	case "count", "sum", "avg", "min", "max", "array_agg", "string_agg", "json_agg", "jsonb_agg", "json_object_agg", "jsonb_object_agg", "bool_and", "bool_or", "every", "bit_and", "bit_or", "bit_xor", "stddev", "stddev_pop", "stddev_samp", "variance", "var_pop", "var_samp", "group_concat":
		return true
	default:
		return false
	}
}

func mergeSQLReferenceSets(dst, src *sqlReferenceSet) {
	if dst == nil || src == nil {
		return
	}
	dst.Tables = append(dst.Tables, src.Tables...)
	dst.Columns = append(dst.Columns, src.Columns...)
	dst.Wildcards = append(dst.Wildcards, src.Wildcards...)
	dst.SelectFields = append(dst.SelectFields, src.SelectFields...)
	if dst.CTEs == nil {
		dst.CTEs = make(map[string]struct{})
	}
	for name := range src.CTEs {
		dst.CTEs[name] = struct{}{}
	}
	dst.HasWildcard = dst.HasWildcard || src.HasWildcard
	dst.HasDistinct = dst.HasDistinct || src.HasDistinct
	dst.HasGroupBy = dst.HasGroupBy || src.HasGroupBy
	dst.HasWindow = dst.HasWindow || src.HasWindow
	dst.HasAggregate = dst.HasAggregate || src.HasAggregate
	dst.HasExplicitLimit = dst.HasExplicitLimit || src.HasExplicitLimit
	dst.PureGlobalAggregate = dst.PureGlobalAggregate || src.PureGlobalAggregate
}

func copyStringSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return map[string]struct{}{}
	}
	clone := make(map[string]struct{}, len(source))
	for key := range source {
		clone[key] = struct{}{}
	}
	return clone
}

func stringSliceToSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		normalized := normalizeIdentifier(item)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}

func uniqueNormalizedStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		normalized := normalizeIdentifier(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func sortedSetKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unionColumnLists(groups ...[]*sqlRelationBinding) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, bindings := range groups {
		for _, binding := range bindings {
			if binding == nil {
				continue
			}
			for _, columnName := range binding.columnList {
				if _, ok := seen[columnName]; ok {
					continue
				}
				seen[columnName] = struct{}{}
				result = append(result, columnName)
			}
		}
	}
	return result
}

func nodeFromWindowDef(windowDef *pg_query.WindowDef) *pg_query.Node {
	if windowDef == nil {
		return nil
	}
	return &pg_query.Node{Node: &pg_query.Node_WindowDef{WindowDef: windowDef}}
}
