package ru

import "testing"

func TestGenitiveFirstName(t *testing.T) {
	cases := map[string]string{
		"Катя":   "Кати",
		"Ира":    "Иры",
		"Марк":   "Марка",
		"Женя":   "Жени",
		"Настя":  "Насти",
		"Игорь":  "Игоря",
		"Андрей": "Андрея",
		"Лена":   "Лены",
		"Али":    "Али",

		"Саша":  "Саши",
		"Ольга": "Ольги",

		"Bob": "Bob",
		"":    "",
	}

	for name, want := range cases {
		if got := GenitiveFirstName(name); got != want {
			t.Errorf("GenitiveFirstName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestInstrumentalFirstName(t *testing.T) {
	cases := map[string]string{
		"Катя":   "Катей",
		"Ира":    "Ирой",
		"Марк":   "Марком",
		"Женя":   "Женей",
		"Настя":  "Настей",
		"Игорь":  "Игорем",
		"Андрей": "Андреем",
		"Лена":   "Леной",
		"Никита": "Никитой",

		"Саша":  "Сашей",
		"Ольга": "Ольгой",
		"Али":   "Али",
		"Bob":   "Bob",
		"":      "",
	}

	for name, want := range cases {
		if got := InstrumentalFirstName(name); got != want {
			t.Errorf("InstrumentalFirstName(%q) = %q, want %q", name, got, want)
		}
	}
}
