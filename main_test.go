package main

import (
	"testing"
)

func TestRedactPHI(t *testing.T) {
	task := Task{
		ID:          "test1",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName":     "Alice",
			"dob":             "10/09/1991",
			"phone":           "2024157890",
			"insuranceNumber": "138928943893",
		},
	}

	concealPHI(&task)

	if task.Payload["patientName"] != "[CONCEALED]" {
		t.Errorf("Expected patient name to be concealed, got %v", task.Payload["patientName"])
	}

	if task.Payload["dob"] != "[CONCEALED]" {
		t.Errorf("Date of Birth should be concealed.")
	}

	if task.Payload["phone"] != "[CONCEALED]" {
		t.Errorf("Phone number should be concealed.")
	}

	if task.Payload["insuranceNumber"] != "[CONCEALED]" {
		t.Errorf("Insurance number should be concealed.")
	}
}

func TestRedactPHI2(t *testing.T) {
	task := Task{
		ID:          "test2",
		ContainsPHI: true,
		Payload: map[string]interface{}{
			"patientName": "Daniel",
			"diagnosis":   "Bronchitis",
			"address":     "2890 Halfview Court San Bruno, CA",
		},
	}

	concealPHI(&task)

	if task.Payload["diagnosis"] != "[CONCEALED]" {
		t.Errorf("Expected patient diagnosis to be concealed, got %v", task.Payload["diagnosis"])
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
