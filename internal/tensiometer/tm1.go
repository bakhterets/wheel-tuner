package tensiometer

// tm1 is the Park Tool TM-1 tensiometer (conversion table for round steel
// spokes, https://www.parktool.com/assets/doc/product/TM-1_conv-table.pdf).
// Readings are the integer deflection scale; each column is a spoke diameter.
func tm1() Model {
	return Model{
		ID:          "park-tool-tm-1",
		Name:        "Park Tool TM-1",
		ReadingUnit: "scale",
		Note:        "Steel round spokes; measure butted spokes on the thin middle section and pick its diameter.",
		Gauges: []Gauge{
			series("1.4", "Round 1.4", 8, 1,
				54, 58, 63, 69, 75, 83, 91, 100, 111, 123, 137, 154, 172),
			series("1.5", "Round 1.5", 9, 1,
				52, 56, 61, 66, 73, 80, 88, 97, 107, 119, 133, 148, 166),
			series("1.6", "Round 1.6", 11, 1,
				54, 58, 64, 70, 76, 84, 93, 103, 114, 128, 143, 160),
			series("1.7", "Round 1.7", 12, 1,
				51, 56, 61, 67, 73, 81, 89, 99, 110, 122, 137, 154, 174),
			series("1.8", "Round 1.8", 14, 1,
				53, 58, 64, 70, 77, 85, 94, 105, 117, 131, 148, 167),
			series("2.0", "Round 2.0", 17, 1,
				53, 58, 63, 70, 77, 86, 96, 107, 120, 135, 153, 173),
			series("2.3", "Round 2.3", 22, 1,
				54, 59, 66, 73, 82, 92, 104, 117, 133, 151, 172),
			series("2.5", "Round 2.5", 25, 1,
				53, 59, 66, 73, 83, 93, 106, 120, 137, 157),
			series("2.6", "Round 2.6", 27, 1,
				55, 62, 69, 78, 88, 100, 114, 130, 150, 172),
		},
	}
}
