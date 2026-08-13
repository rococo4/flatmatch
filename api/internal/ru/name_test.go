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
