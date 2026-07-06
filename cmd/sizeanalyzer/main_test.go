package main

import (
	"reflect"
	"testing"
)

func TestStringListAcceptsRepeatedValues(t *testing.T) {
	var values stringList
	if err := values.Set("build"); err != nil {
		t.Fatal(err)
	}
	if err := values.Set("app/dist"); err != nil {
		t.Fatal(err)
	}
	if want := (stringList{"build", "app/dist"}); !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
}
