// c6-q-rules: a questionnaire item's validator as rules (gk.QuestionnaireRules,
// QuestionnaireItemDefinition.WithValidator, QuestionnaireItemDefinitionRead.Validator).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func init() {
	modes["c6-q-rules"] = &mode{click: once(qRules)}
}

// A questionnaire item's validator as rules: two choices where one is allowed is refused with the
// app's message, one is valid; the definition's validator read back.
func qRules(a *App, cx *gk.Cx) {
	color := gk.NewQuestionnaireItemDefinition("color", "Favourite colour").WithMultiple(true).
		WithChoice(gk.NewQuestionnaireChoiceDefinition("red", "Red")).WithChoice(gk.NewQuestionnaireChoiceDefinition("blue", "Blue")).
		WithValidator(gk.QuestionnaireRules{MaxChoices: 1, Message: "{item}: one only"})
	plain := gk.NewQuestionnaireItemDefinition("plain", "No rules").
		WithChoice(gk.NewQuestionnaireChoiceDefinition("x", "X"))
	q := gk.NewQuestionnaireState(cx, []gk.QuestionnaireItemDefinition{color, plain}).Build()
	_ = q.SetAnswer(cx, "color", gk.NewQuestionnaireAnswer().WithChoices([]string{"red", "blue"}))
	_ = q.GoNext(cx)
	printError(q, cx, "two")
	_ = q.SetAnswer(cx, "color", gk.NewQuestionnaireAnswer().WithChoices([]string{"blue"}))
	_ = q.GoNext(cx)
	printError(q, cx, "one")
	has, phas := false, false
	if def, dok := q.ItemDefinition(cx, "color").Get(); dok {
		has = def.Validator(cx).Valid()
	}
	if pdef, pok := q.ItemDefinition(cx, "plain").Get(); pok {
		phas = pdef.Validator(cx).Valid()
	}
	fmt.Printf("values c6 rules validator color=%v plain=%v\n", has, phas)
}

func printError(q gk.QuestionnaireState, cx *gk.Cx, what string) {
	e, ok := q.Error(cx, "color").Get()
	if !ok {
		fmt.Printf("values c6 rules %s valid\n", what)
		return
	}
	switch x := e.(type) {
	case gk.QuestionnaireValidationErrorMessage:
		fmt.Printf("values c6 rules %s message=%q\n", what, x.Value)
	default:
		fmt.Printf("values c6 rules %s other=%v\n", what, e)
	}
}
