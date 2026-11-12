package solver

// Matrix is a dense row-major matrix.
type Matrix struct {
	rows, cols int
	data       []float64
}

// NewMatrix returns a zero rows×cols matrix.
func NewMatrix(rows, cols int) *Matrix {
	return &Matrix{rows: rows, cols: cols, data: make([]float64, rows*cols)}
}

// Dims returns the number of rows and columns.
func (m *Matrix) Dims() (rows, cols int) { return m.rows, m.cols }

// At returns the element at row i, column j.
func (m *Matrix) At(i, j int) float64 { return m.data[i*m.cols+j] }

// Set assigns the element at row i, column j.
func (m *Matrix) Set(i, j int, v float64) { m.data[i*m.cols+j] = v }

// Row returns a copy of row i.
func (m *Matrix) Row(i int) []float64 {
	r := make([]float64, m.cols)
	copy(r, m.row(i))
	return r
}

func (m *Matrix) row(i int) []float64 { return m.data[i*m.cols : (i+1)*m.cols] }

// MulVec returns m·x.
func (m *Matrix) MulVec(x []float64) []float64 {
	out := make([]float64, m.rows)
	for i := range out {
		out[i] = dot(m.row(i), x)
	}
	return out
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// stack concatenates matrices with equal column counts vertically.
func stack(ms ...*Matrix) *Matrix {
	rows := 0
	for _, m := range ms {
		rows += m.rows
	}
	out := &Matrix{rows: rows, cols: ms[0].cols, data: make([]float64, 0, rows*ms[0].cols)}
	for _, m := range ms {
		out.data = append(out.data, m.data...)
	}
	return out
}
