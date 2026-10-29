package programme

import "testing"

func TestShapeCountsOnlyNumbers(t *testing.T) {
	st, _ := newStore(t)
	p := mustProg(t, st, 1, 2032, 3)
	fp, _ := st.GetFullProgramme(1, p.ID)
	var parade Day
	for _, d := range fp.Days {
		if d.IsParade {
			parade = d
			break
		}
	}
	st.SaveDaysAndRows(1, p.ID, []DayInput{{ID: parade.ID, IsParade: true, UniformKey: "green", Notes: "bring water",
		Rows: []RowInput{{ID: parade.Rows[0].ID, Texts: []string{"First aid with Sgt Secret", "", "Drill", ""}}}}})
	st.AddClosure(p.ID, "2032-03-01", "2032-03-02", "x")
	st.AddWeekendNote(p.ID, "2032-03-06", "camp")
	c, _ := st.AddRequirementCategory(p.ID, "Fieldcraft", "")
	st.AddRequirementEntry(p.ID, c, "Cdt Secret", "n")
	st.AddRequirementEntry(p.ID, c, "Cdt Other", "n")

	got, _ := st.GetFullProgramme(1, p.ID)
	sh := got.Shape()
	if sh.Year != 2032 || sh.Month != 3 || sh.Days != 31 {
		t.Errorf("identity: %+v", sh)
	}
	if sh.ParadeNights < 1 || sh.NightsWithNotes != 1 || sh.Closures != 1 || sh.WeekendNotes != 1 || sh.RequirementCategory != 1 || sh.RequirementEntries != 2 || sh.DistinctUniformKeys != 1 {
		t.Errorf("counts: %+v", sh)
	}
	if sh.FilledCells != 2 || sh.LongestCellRunes != len([]rune("First aid with Sgt Secret")) || sh.Cells < 4 {
		t.Errorf("cells: %+v", sh)
	}
}

func TestSchemaInfo(t *testing.T) {
	st, _ := newStore(t)
	n, newest := st.SchemaInfo()
	if n < 9 || newest == "" {
		t.Errorf("SchemaInfo = %d, %q; want the applied migrations", n, newest)
	}
}
