package checkers

import (
	"fmt"
	"html"
	"io"
	"os"

	"github.com/couchbaselabs/gorgon/src/gorgon"
)

func VisualizeSequentialPath(m gorgon.Model, history [][]gorgon.Operation, sequence []int, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = VisualizeSequential(m, history, sequence, f)
	clErr := f.Close()
	if err != nil {
		return err
	}
	return clErr
}

func VisualizeSequential(m gorgon.Model, history [][]gorgon.Operation, sequence []int, writer io.Writer) error {
	const header = `<!DOCTYPE html>
<html>
<head>
<title>Sequential Visualization</title>
<style>
table { border-collapse: collapse; }
td { border: 1px solid; }
.grey { background: #eee; }
</style>
</head>
<body>
<p><a href="#first-unsequenced">First unsequenced</a></p>
<table>
<thead><tr>`
	empty := []byte("<td></td>")
	trBegin := []byte("<tr>")
	trEnd := []byte("</tr>\n")
	trGrey := []byte("<tr class=\"grey\">")
	_, err := writer.Write([]byte(header))
	if err != nil {
		return err
	}
	for i := range history {
		_, err = fmt.Fprintf(writer, "<th>Client %d</th>", i)
		if err != nil {
			return err
		}
	}
	_, err = writer.Write([]byte("</tr></thead>\n<tbody>\n"))
	if err != nil {
		return err
	}
	remaining := make([][]gorgon.Operation, len(history))
	copy(remaining, history)
	state := m.Init()[0]
	for _, t := range sequence {
		_, err = writer.Write(trBegin)
		if err != nil {
			return err
		}
		for i := range history {
			if i != t {
				_, err = writer.Write(empty)
				if err != nil {
					return err
				}
				continue
			}
			input := remaining[i][0].Input
			output := remaining[i][0].Output
			_, err = fmt.Fprintf(writer, "<td title=\"%v\">%v</td>",
				html.EscapeString(m.DescribeState(state)),
				html.EscapeString(m.DescribeOperation(input, output)))
			if err != nil {
				return err
			}
			state = m.Step(state, input, output)[0]
		}
		_, err = writer.Write(trEnd)
		if err != nil {
			return err
		}
		remaining[t] = remaining[t][1:]
	}
	first := true
	for {
		done := true
		for i := range remaining {
			if len(remaining[i]) > 0 {
				done = false
				break
			}
		}
		if done {
			break
		}
		if first {
			first = false
			_, err = writer.Write([]byte(`<tr class="grey" id="first-unsequenced">`))
		} else {
			_, err = writer.Write(trGrey)
		}
		if err != nil {
			return err
		}
		for i := range history {
			if len(remaining[i]) == 0 {
				_, err = writer.Write(empty)
				if err != nil {
					return err
				}
				continue
			}
			_, err = fmt.Fprintf(writer, "<td>%v</td>", html.EscapeString(
				m.DescribeOperation(remaining[i][0].Input, remaining[i][0].Output)))
			if err != nil {
				return err
			}
			remaining[i] = remaining[i][1:]
		}
		_, err = writer.Write(trEnd)
		if err != nil {
			return err
		}
	}
	var last []byte
	last = append(last, "</tbody>\n</table>\n"...)
	if first {
		last = append(last, "<p id=\"first-unsequenced\">All operations were sequenced.</p>\n"...)
	}
	last = append(last, "</body>\n</html>\n"...)
	_, err = writer.Write(last)
	return err
}
