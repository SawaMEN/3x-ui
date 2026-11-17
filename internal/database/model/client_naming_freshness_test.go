package model

import "testing"

func TestMergeClientRecordCommentDeletionSurvivesOlderSnapshots(t *testing.T) {
	row := &ClientRecord{Comment: "old label", UpdatedAt: 100}
	MergeClientRecord(row, &ClientRecord{Comment: "", UpdatedAt: 200})
	if row.Comment != "" {
		t.Fatal("newer explicit deletion did not clear the comment")
	}
	row.UpdatedAt = 200
	MergeClientRecord(row, &ClientRecord{Comment: "old label", UpdatedAt: 100})
	if row.Comment != "" {
		t.Fatal("an older snapshot resurrected the deleted comment")
	}
}
