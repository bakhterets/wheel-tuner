package tensiometer

// tc02 is the ZTTO TC-02 dial tensiometer. Readings are the dial value in mm;
// each column of the manufacturer's table is a spoke profile
// (end-middle-end diameters in mm for butted spokes).
func tc02() Model {
	return Model{
		ID:          "ztto-tc-02",
		Name:        "ZTTO TC-02",
		ReadingUnit: "mm",
		Note:        "Measure butted spokes on the thin middle section.",
		Gauges: []Gauge{
			series("1.8-1.5-1.8", "Butted 1.8-1.5-1.8", 1.1, 0.1,
				53, 56, 60, 64, 67, 74, 77, 83, 88, 93, 99, 105, 110, 117, 126, 133, 140, 152),
			series("1.8-1.6-1.8", "Butted 1.8-1.6-1.8", 1.4, 0.1,
				53, 57, 60, 62, 68, 72, 77, 83, 89, 93, 100, 105, 111, 118, 123, 133, 144, 151),
			series("2.0-1.7-1.8", "Butted 2.0-1.7-1.8", 1.5, 0.1,
				53, 55, 59, 64, 68, 71, 77, 82, 86, 91, 98, 105, 111, 118, 127, 135, 141, 152),
			series("2.0-1.8-2.0", "Butted 2.0-1.8-2.0", 1.7, 0.1,
				50, 53, 57, 62, 65, 70, 73, 78, 83, 87, 93, 102, 108, 115, 120, 127, 140, 147),
			series("1.8", "Straight 1.8", 1.8, 0.1,
				50, 54, 58, 62, 66, 71, 77, 83, 88, 94, 100, 108, 113, 121, 126, 137, 148),
			series("2.0", "Straight 2.0", 2.4, 0.1,
				51, 55, 61, 66, 70, 75, 79, 84, 92, 97, 105, 110, 117, 126, 137, 146, 155),
			series("2.3", "Straight 2.3", 3.0, 0.1,
				50, 53, 56, 58, 62, 67, 71, 78, 84, 89, 94, 100, 107, 113, 120, 131, 139, 150),
		},
	}
}
