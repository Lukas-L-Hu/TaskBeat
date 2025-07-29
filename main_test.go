package main

import (
	"testing"
)

func TestRedactPHI(t *testing.T) {
	task := Task{
		ID:          "test1",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Alice",
			"otherField":  "value",
		},
	}

	concealPHI(&task)

	if task.Payload["patientName"] != "[CONCEALED]" {
		t.Errorf("Expected patient name to be concealed, got %v", task.Payload["patientName"])
	}

	if task.Payload["otherField"] != "value" {
		t.Errorf("Other fields should not be modified")
	}
}

func TestRedactPHI_NoPHI(t *testing.T) {
	task := Task{
		ID:          "test2",
		ContainsPHI: false,
		Payload: map[string]interface{}{
			"patientName": "Bob",
		},
	}

	concealPHI(&task)

	if task.Payload["patientName"] != "Bob" {
		t.Errorf("Expected patientName to remain unchanged, got %v", task.Payload["patientName"])
	}
}
