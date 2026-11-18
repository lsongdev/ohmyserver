package modem

import "testing"

func TestServingCells(t *testing.T) {
	tests := []struct {
		name                   string
		lines                  []string
		radio                  string
		band, rsrp, rsrq, sinr int
		index                  int
	}{
		{"SA", []string{`+QENG: "servingcell","NOCONN","NR5G-SA","TDD",001,01,000000001,1,00001,627264,78,3,-82,-11,16,1,32`}, "5G SA", 78, -82, -11, 16, 0},
		{"LTE", []string{`+QENG: "servingcell","NOCONN","LTE","FDD",001,01,0000001,1,1850,3,5,5,0001,-99,-13,-69,12,6,0,20`}, "LTE", 3, -99, -13, 12, 0},
		{"NSA", []string{`+QENG: "servingcell","NOCONN"`, `+QENG: "LTE","FDD",001,01,0000001,1,1850,3,5,5,0001,-99,-13,-69,12,6,0,20`, `+QENG: "NR5G-NSA",001,01,1,-101,21,-10,640320,78,7,1`}, "5G NSA", 78, -101, -10, 21, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, cells := servingCells(test.lines)
			if state != "NOCONN" || len(cells) != test.index+1 {
				t.Fatalf("state=%q, cells=%+v", state, cells)
			}
			cell := cells[test.index]
			if cell.Radio != test.radio {
				t.Errorf("radio=%q, want %q", cell.Radio, test.radio)
			}
			for _, metric := range []struct {
				name string
				got  *int
				want int
			}{
				{"band", cell.Band, test.band}, {"RSRP", cell.RSRP, test.rsrp},
				{"RSRQ", cell.RSRQ, test.rsrq}, {"SINR", cell.SINR, test.sinr},
			} {
				if metric.got == nil {
					t.Errorf("%s absent", metric.name)
				} else if *metric.got != metric.want {
					t.Errorf("%s=%d, want %d", metric.name, *metric.got, metric.want)
				}
			}
		})
	}
}

func TestUnavailableCells(t *testing.T) {
	state, cells := servingCells([]string{`+QENG: "servingcell","SEARCH"`, `+QENG: "NR5G-NSA",001,01,65535,-32768,-32768,-32768`})
	if state != "SEARCH" || len(cells) != 0 {
		t.Fatalf("state=%q, cells=%+v", state, cells)
	}
}

func TestRegistration(t *testing.T) {
	if status := registration(`+C5GREG: 0,1`); status == nil || *status != 1 {
		t.Fatalf("expected home registration, got %v", status)
	}
	if status := registration(`+C5GREG: 1,"0001","000000001",11`); status != nil {
		t.Fatalf("accepted unsolicited result: %d", *status)
	}
}
