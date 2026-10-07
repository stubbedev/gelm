package highlight

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stubbedev/gelm/widget"
)

// show shows a line's spans as class:text pairs.
func show(line string, spans []widget.TextSpan) string {
	rs := []rune(line)
	parts := make([]string, len(spans))
	for i, s := range spans {
		parts[i] = fmt.Sprintf("%s:%s", strings.TrimPrefix(s.Class, "def:"), string(rs[s.Start:s.End]))
	}
	return strings.Join(parts, " ")
}

func TestTOMLLines(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{`# a comment`, `comment:# a comment`},
		{`[modules.custom]`, `keyword:[modules.custom]`},
		{`  [[custom]] # each module`, `keyword:[[custom]] comment:# each module`},
		{`[table] junk`, `keyword:[table] error:junk`},
		{`name = "cpu"`, `type:name string:"cpu"`},
		{`a.b-c = 'lit\n'`, `type:a type:b-c string:'lit\n'`},
		{`"quoted key" = true`, `type:"quoted key" boolean:true`},
		{`esc = "tab\there \u00e9!"`, `type:esc string:"tab special-char:\t string:here  special-char:\u00e9 string:!"`},
		{`n = 42`, `type:n decimal:42`},
		{`n = -1_000`, `type:n decimal:-1_000`},
		{`h = 0xff`, `type:h decimal:0xff`},
		{`f = 3.14`, `type:f floating-point:3.14`},
		{`f = 1e9`, `type:f floating-point:1e9`},
		{`f = -inf`, `type:f floating-point:-inf`},
		{`d = 1979-05-27T07:32:00Z`, `type:d constant:1979-05-27T07:32:00Z`},
		{`t = 07:32:00`, `type:t constant:07:32:00`},
		{`b = falsehood`, `type:b error:f error:a error:l error:s error:e error:h error:o error:o error:d`},
		{`arr = [1, "two", true] # tail`, `type:arr decimal:1 string:"two" boolean:true comment:# tail`},
		{`inline = { x = 1, "y" = 'z' }`, `type:inline type:x decimal:1 type:"y" string:'z'`},
	} {
		spans, end := TOML{}.Highlight([]rune(tc.line), 0)
		if got := show(tc.line, spans); got != tc.want {
			t.Errorf("%s\n got %s\nwant %s", tc.line, got, tc.want)
		}
		if end != 0 {
			t.Errorf("%s: ends in state %d, want 0", tc.line, end)
		}
	}
}

// Multi-line strings and arrays carry over to the lines after them.
func TestTOMLStateCarries(t *testing.T) {
	doc := []string{
		`cmd = """first`,
		`second \n`,
		`last""" # done`,
		`items = [`,
		`  "a", { k = 1 },`,
		`]`,
		`lit = '''raw \n`,
		`still'''`,
		`after = 1`,
	}
	want := []string{
		`type:cmd string:"""first`,
		`string:second  special-char:\n`,
		`string:last""" comment:# done`,
		`type:items`,
		`string:"a" type:k decimal:1`,
		``,
		`type:lit string:'''raw \n`,
		`string:still'''`,
		`type:after decimal:1`,
	}
	state := 0
	for i, line := range doc {
		var spans []widget.TextSpan
		spans, state = TOML{}.Highlight([]rune(line), state)
		if got := show(line, spans); got != want[i] {
			t.Errorf("line %d %q\n got %s\nwant %s", i, line, got, want[i])
		}
	}
	if state != 0 {
		t.Errorf("the document ends in state %d", state)
	}
}

func TestTOMLStateRoundTrips(t *testing.T) {
	st := tomlState{mode: modeBasic, stack: []bool{false, true, false}, key: true}
	got := decodeState(st.encode())
	if got.mode != st.mode || got.key != st.key || fmt.Sprint(got.stack) != fmt.Sprint(st.stack) {
		t.Errorf("state %+v round-trips as %+v", st, got)
	}
	if decodeState(0).mode != modeNone || len(decodeState(0).stack) != 0 {
		t.Error("state 0 is not the plain start")
	}
}
