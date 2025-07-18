package ui

import "fmt"

const brailleBase = 0x2800

func SetChunkState(chunk [][8]bool, index int, flag bool) {
	if len(chunk)*8 <= index {
		return
	}

	chunk[index/8][index%8] = flag
}

func genText(chunks [][8]bool) (string, int) {
	var (
		text string
		line int = 1
	)
	for i, c := range chunks {
		text += ChunksToText(c)

		if len(chunks)*8 >= 5000 {
			if (i+1)%(len(chunks)*8/100) == 0 {
				text += "\n"
				line++
			}
		} else {
			if (i+1)%50 == 0 {
				text += "\n"
				line++
			}
		}
	}

	return text, line
}

func MakeChunks(num int) [][8]bool {
	chunkNum := (num + 7) / 8
	return make([][8]bool, chunkNum)
}

func ChunksToText(chunks [8]bool) string {
	chunks = [8]bool{chunks[0], chunks[1], chunks[2], chunks[6], chunks[3], chunks[4], chunks[5], chunks[7]}
	char := 0
	for i, c := range chunks {
		if c {
			char |= 1 << i
		} else {
			char |= 0 << i
		}
	}
	return string(rune(brailleBase + char))
}

func UpdateState(chunks [][8]bool, index int, flag bool) {
	SetChunkState(chunks, index, flag)

	text, lines := genText(chunks)
	fmt.Printf("\033[%dA", lines)
	fmt.Println(text)
}

func ClearState(chunks [][8]bool) {
	_, lines := genText(chunks)
	for i := 0; i < lines; i++ {
		fmt.Printf("\x1b[1A")
		fmt.Printf("\x1b[2K")
	}
	fmt.Printf("\n")
}
