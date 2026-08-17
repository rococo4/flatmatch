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
		// Шипящая перед -а: «Саши», не «Сашы».
		"Саша":  "Саши",
		"Ольга": "Ольги",
		// Чужое имя выдумывать нечем — оно едет как есть.
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
		// Шипящая перед -а: «Сашей», не «Сашой»; г/к/х — обычная согласная.
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
