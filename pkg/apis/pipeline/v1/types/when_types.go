/*
Copyright 2025 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"fmt"

	"github.com/tektoncd/pipeline/pkg/substitution"
	"k8s.io/apimachinery/pkg/selection"
)

// WhenExpression allows a PipelineTask to declare expressions to be evaluated before the Task is run
// to determine whether the Task should be executed or skipped
type WhenExpression struct {
	// Input is the string for guard checking which can be a static input or an output from a parent Task
	Input string `json:"input,omitempty"`

	// Operator that represents an Input's relationship to the values
	Operator selection.Operator `json:"operator,omitempty"`

	// Values is an array of strings, which is compared against the input, for guard checking
	// It must be non-empty
	// +listType=atomic
	Values []string `json:"values,omitempty"`

	// CEL is a string of Common Language Expression, which can be used to conditionally execute
	// the task based on the result of the expression evaluation
	// More info about CEL syntax: https://github.com/google/cel-spec/blob/master/doc/langdef.md
	// +optional
	CEL string `json:"cel,omitempty"`
}

func (we *WhenExpression) isInputInValues() bool {
	for i := range we.Values {
		if we.Values[i] == we.Input {
			return true
		}
	}
	return false
}

func (we *WhenExpression) isTrue() bool {
	if we.Operator == selection.In {
		return we.isInputInValues()
	}
	// selection.NotIn
	return !we.isInputInValues()
}

func (we *WhenExpression) applyReplacements(replacements map[string]string, arrayReplacements map[string][]string) WhenExpression {
	replacedInput := substitution.ApplyReplacements(we.Input, replacements)
	replacedCEL := substitution.ApplyReplacements(we.CEL, replacements)

	var replacedValues []string
	for _, val := range we.Values {
		// arrayReplacements holds a list of array parameters with a pattern - params.arrayParam1
		// array params are referenced using $(params.arrayParam1[*])
		// array results are referenced using $(results.resultname[*])
		// check if the param exist in the arrayReplacements to replace it with a list of values
		if _, ok := arrayReplacements[fmt.Sprintf("%s.%s", ParamsPrefix, ArrayReference(val))]; ok {
			replacedValues = append(replacedValues, substitution.ApplyArrayReplacements(val, replacements, arrayReplacements)...)
		} else if _, ok := arrayReplacements[ResultsArrayReference(val)]; ok {
			replacedValues = append(replacedValues, substitution.ApplyArrayReplacements(val, replacements, arrayReplacements)...)
		} else {
			replacedValues = append(replacedValues, substitution.ApplyReplacements(val, replacements))
		}
	}

	return WhenExpression{Input: replacedInput, Operator: we.Operator, Values: replacedValues, CEL: replacedCEL}
}

// GetVarSubstitutionExpressions extracts all the values between "$(" and ")" in a When Expression
func (we *WhenExpression) GetVarSubstitutionExpressions() ([]string, bool) {
	var allExpressions []string
	allExpressions = append(allExpressions, validateString(we.Input)...)
	allExpressions = append(allExpressions, validateString(we.CEL)...)
	for _, value := range we.Values {
		allExpressions = append(allExpressions, validateString(value)...)
	}
	return allExpressions, len(allExpressions) != 0
}

// WhenExpressions are used to specify whether a Task should be executed or skipped
// All of them need to evaluate to True for a guarded Task to be executed.
type WhenExpressions []WhenExpression

type StepWhenExpressions = WhenExpressions

// AllowsExecution evaluates an Input's relationship to an array of Values, based on the Operator,
// to determine whether all the When Expressions are True. If they are all True, the guarded Task is
// executed, otherwise it is skipped.
// If CEL expression exists, AllowsExecution will get the evaluated results from evaluatedCEL and determine
// if the Task should be skipped.
func (wes WhenExpressions) AllowsExecution(evaluatedCEL map[string]bool) bool {
	for _, we := range wes {
		if !we.isTrue() || (we.CEL != "" && !evaluatedCEL[we.CEL]) {
			return false
		}
	}
	return true
}

// ReplaceVariables interpolates variables, such as Parameters and Results, in
// the Input and Values.
func (wes WhenExpressions) ReplaceVariables(replacements map[string]string, arrayReplacements map[string][]string) WhenExpressions {
	replaced := wes
	for i := range wes {
		replaced[i] = wes[i].applyReplacements(replacements, arrayReplacements)
	}
	return replaced
}
