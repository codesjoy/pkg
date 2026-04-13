// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

import "sort"

// Condition represents a WHERE clause condition extracted from the filter AST.
// It contains the column being filtered and whether it's an equality condition.
type Condition struct {
	Column     *Column
	IsEquality bool
}

// reorderedExpressionFactors attempts to reorder WHERE clause factors to match
// the prefix of a composite index, improving query performance.
//
// Pipeline:
//  1. Extract conditions: Identify which factors are simple equality/comparison
//     conditions on filterable columns.
//  2. Find best index: Score each composite index based on how many prefix
//     columns are covered by conditions.
//  3. Reorder: Sort conditions into equality-first, then range, then other,
//     each sub-sorted by index column order.
//  4. Reconstruct: Build a new factor list preserving non-condition factors'
//     relative positions while placing reordered conditions first.
//
// Returns (nil, false) if no beneficial reordering is found.
func (w *whereClause) reorderedExpressionFactors(factors []*Factor) ([]*Factor, bool) {
	conditions, factorConditionIndexes, err := w.extractConditionsFromFactors(factors)
	if err != nil {
		return nil, false
	}

	bestIndex := findBestCompositeIndex(conditions, w.table.CompositeIndexes)
	if bestIndex == nil {
		return nil, false
	}

	reorderedConditions := reorderConditions(conditions, bestIndex)
	if sameConditionOrder(conditions, reorderedConditions) {
		return nil, false
	}

	return w.reorderFactors(factors, conditions, factorConditionIndexes, reorderedConditions), true
}

// extractConditionsFromFactors walks the flat factor list and extracts conditions
// that are eligible for index optimization. A factor is eligible if:
//   - It has exactly one term (no OR)
//   - The term is not negated
//   - It is a simple restriction on a filterable, non-key-value column
//   - The comparator is one of: =, <, >, <=, >=, or : (with exact match mode)
//
// Returns the extracted conditions and a mapping from factor index to condition index.
func (w *whereClause) extractConditionsFromFactors(
	factors []*Factor,
) ([]Condition, map[int]int, error) {
	conditions := make([]Condition, 0, len(factors))
	factorConditionIndexes := make(map[int]int, len(factors))

	for i, factor := range factors {
		if len(factor.Terms) != 1 {
			continue
		}
		term := factor.Terms[0]
		if term.Negated || term.Simple == nil || term.Simple.Restriction == nil {
			continue
		}
		restriction := term.Simple.Restriction
		if restriction.Comparable == nil || restriction.Comparable.Member == nil {
			continue
		}
		if len(restriction.Comparable.Member.Fields) > 0 {
			continue
		}

		column, err := w.table.FilterableColumnByFieldPath(
			NewFieldPath(restriction.Comparable.Member.Value),
		)
		if err != nil || column.keyValue {
			continue
		}

		isEquality, ok := w.restrictionEquality(column, restriction)
		if !ok {
			continue
		}

		conditionIndex := len(conditions)
		conditions = append(conditions, Condition{
			Column:     column,
			IsEquality: isEquality,
		})
		factorConditionIndexes[i] = conditionIndex
	}

	return conditions, factorConditionIndexes, nil
}

// restrictionEquality classifies a restriction as equality or range.
// Returns (isEquality, ok) where ok is false if the restriction is not indexable.
func (w *whereClause) restrictionEquality(column *Column, restriction *Restriction) (bool, bool) {
	switch restriction.Comparator {
	case "=":
		return true, true
	case ">", "<", ">=", "<=":
		return false, true
	case ":":
		if err := validateHasOperator(column); err != nil {
			return false, false
		}
		mode, err := w.selectMatchMode(column, true)
		if err != nil {
			return false, false
		}
		return mode == MatchModeExact, true
	default:
		return false, false
	}
}

// reorderFactors rebuilds the factor list so that index-matching conditions appear
// first (in their reordered positions), while non-condition factors retain their
// original relative order at the end.
//
// Position mapping:
//   - Condition factors get positions 0..N-1 based on their reordered index.
//   - Non-condition factors get positions N, N+1, ... based on original factor index,
//     ensuring they appear after all conditions.
func (w *whereClause) reorderFactors(
	factors []*Factor,
	conditions []Condition,
	factorConditionIndexes map[int]int,
	reorderedConditions []Condition,
) []*Factor {
	conditionPositions := make(map[int]int, len(conditions))
	usedReordered := make([]bool, len(reorderedConditions))
	for originalIndex, condition := range conditions {
		for reorderedIndex, reorderedCondition := range reorderedConditions {
			if usedReordered[reorderedIndex] {
				continue
			}
			if condition.Column.databaseName == reorderedCondition.Column.databaseName &&
				condition.IsEquality == reorderedCondition.IsEquality {
				conditionPositions[originalIndex] = reorderedIndex
				usedReordered[reorderedIndex] = true
				break
			}
		}
	}

	type factorWithPosition struct {
		factor   *Factor
		position int
	}

	factorsWithPositions := make([]factorWithPosition, 0, len(factors))
	for i, factor := range factors {
		if conditionIndex, ok := factorConditionIndexes[i]; ok {
			if position, hasPosition := conditionPositions[conditionIndex]; hasPosition {
				factorsWithPositions = append(factorsWithPositions, factorWithPosition{
					factor:   factor,
					position: position,
				})
				continue
			}
		}
		factorsWithPositions = append(factorsWithPositions, factorWithPosition{
			factor:   factor,
			position: len(reorderedConditions) + i,
		})
	}

	sort.SliceStable(factorsWithPositions, func(i, j int) bool {
		return factorsWithPositions[i].position < factorsWithPositions[j].position
	})

	reorderedFactors := make([]*Factor, len(factors))
	for i, positioned := range factorsWithPositions {
		reorderedFactors[i] = positioned.factor
	}
	return reorderedFactors
}

// findBestCompositeIndex returns the index with the highest score, or nil if no
// index covers at least one condition prefix column.
func findBestCompositeIndex(conditions []Condition, indexes []CompositeIndex) *CompositeIndex {
	if len(indexes) == 0 || len(conditions) == 0 {
		return nil
	}

	var bestIndex *CompositeIndex
	bestScore := 0
	for i := range indexes {
		score := calculateIndexScore(conditions, indexes[i])
		if score <= bestScore {
			continue
		}
		bestScore = score
		bestIndex = &indexes[i]
	}
	return bestIndex
}

// calculateIndexScore computes a heuristic score for how well the conditions match
// the index prefix. The scoring formula rewards:
//
//	Prefix coverage: for each consecutive index column matched from position 0,
//	                 add (indexLen - position) * 10. Earlier columns score higher.
//	Equality bonus:  add 5 if the condition is an equality (=), since equality
//	                 conditions narrow the search space more efficiently than range conditions.
//
// The score is 0 if the first index column is not covered (no prefix match at all).
// Example: index (a, b, c), conditions {a=1, c>5} → score = 30 + 5 = 35
// (only 'a' matches, since 'b' is missing the prefix is broken).
func calculateIndexScore(conditions []Condition, index CompositeIndex) int {
	if len(index.Columns) == 0 || len(conditions) == 0 {
		return 0
	}

	conditionColumns := make(map[string]*Condition)
	for i := range conditions {
		conditionColumns[conditions[i].Column.databaseName] = &conditions[i]
	}

	score := 0
	indexLen := len(index.Columns)
	for i, indexColumn := range index.Columns {
		condition, found := conditionColumns[indexColumn]
		if !found {
			// Prefix is broken — stop scoring. Only a contiguous prefix matters.
			break
		}

		// Prefix column weight: earlier columns are worth more.
		score += (indexLen - i) * 10
		// Equality is more selective than range; give it a bonus.
		if condition.IsEquality {
			score += 5
		}
	}
	return score
}

// reorderConditions sorts conditions using a 3-bucket strategy optimized for
// composite index utilization:
//
//  1. Equality conditions on index columns (sorted by index column order)
//  2. Range conditions on index columns (sorted by index column order)
//  3. All other conditions (preserving original order)
//
// This order allows the database optimizer to efficiently narrow using equality
// lookups on the index prefix, then apply range scans, then filter the remainder.
func reorderConditions(conditions []Condition, index *CompositeIndex) []Condition {
	if index == nil || len(conditions) < 2 {
		return conditions
	}

	indexPositions := make(map[string]int)
	for i, column := range index.Columns {
		indexPositions[column] = i
	}

	// Bucket 1: equality conditions on indexed columns.
	equalityConditions := make([]Condition, 0, len(conditions))
	// Bucket 2: range conditions on indexed columns.
	rangeConditions := make([]Condition, 0, len(conditions))
	// Bucket 3: everything else (non-indexed columns).
	otherConditions := make([]Condition, 0, len(conditions))

	for _, condition := range conditions {
		_, inIndex := indexPositions[condition.Column.databaseName]
		switch {
		case condition.IsEquality && inIndex:
			equalityConditions = append(equalityConditions, condition)
		case !condition.IsEquality && inIndex:
			rangeConditions = append(rangeConditions, condition)
		default:
			otherConditions = append(otherConditions, condition)
		}
	}

	// Sort each indexed bucket by index column order for prefix utilization.
	sortByIndexOrder(equalityConditions, indexPositions)
	sortByIndexOrder(rangeConditions, indexPositions)

	result := make([]Condition, 0, len(conditions))
	result = append(result, equalityConditions...)
	result = append(result, rangeConditions...)
	result = append(result, otherConditions...)
	return result
}

func sortByIndexOrder(conditions []Condition, indexPositions map[string]int) {
	sort.Slice(conditions, func(i, j int) bool {
		pos1, found1 := indexPositions[conditions[i].Column.databaseName]
		pos2, found2 := indexPositions[conditions[j].Column.databaseName]
		if !found1 {
			pos1 = 1<<31 - 1
		}
		if !found2 {
			pos2 = 1<<31 - 1
		}
		return pos1 < pos2
	})
}

func sameConditionOrder(a, b []Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Column.databaseName != b[i].Column.databaseName ||
			a[i].IsEquality != b[i].IsEquality {
			return false
		}
	}
	return true
}
