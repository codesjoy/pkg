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
			break
		}

		score += (indexLen - i) * 10
		if condition.IsEquality {
			score += 5
		}
	}
	return score
}

func reorderConditions(conditions []Condition, index *CompositeIndex) []Condition {
	if index == nil || len(conditions) < 2 {
		return conditions
	}

	indexPositions := make(map[string]int)
	for i, column := range index.Columns {
		indexPositions[column] = i
	}

	equalityConditions := make([]Condition, 0, len(conditions))
	rangeConditions := make([]Condition, 0, len(conditions))
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
