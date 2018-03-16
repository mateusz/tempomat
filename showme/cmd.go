package main

import (
	"github.com/hajimehoshi/ebiten"
	"log"
	"net/rpc"
	"github.com/mateusz/tempomat/api"
	"sort"
	"image"
	"github.com/hajimehoshi/ebiten/ebitenutil"
	"fmt"
	"time"
	"github.com/lucasb-eyer/go-colorful"
	"golang.org/x/image/draw"
)

const (
	tickPeriod = 100 * time.Millisecond
)

var (
	bufPipe chan *image.RGBA
	backBuf *image.RGBA
	palette []colorful.Color
	palPointer int
	palMap map[string]colorful.Color
	scrWidth int
	scrHeight int
	tempomatClient *rpc.Client
)

func main() {
	scrWidth = 1500
	scrHeight = 500
	bufPipe = make(chan *image.RGBA)
	backBuf = image.NewRGBA(image.Rect(0, 0, scrWidth, scrHeight))

	var err error
	palette, err = colorful.SoftPaletteEx(10, colorful.SoftPaletteSettings{isNice, 50, true})
	if err!=nil {
		log.Fatalf("%s", err)
	}

	palMap = make(map[string]colorful.Color, 0)
	palPointer = 0

	tempomatClient, err = rpc.DialHTTP("tcp", "127.0.0.1:29999")
	defer tempomatClient.Close()
	if err != nil {
		log.Fatalf("Failed to dial server: %s", err)
	}

	shutdown := make(chan bool)
	go func(bufPipe chan *image.RGBA, shutdown chan bool) {
		buf := image.NewRGBA(image.Rect(0, 0, scrWidth, scrHeight))
		tick := time.NewTicker(tickPeriod)
		defer tick.Stop()
		for {
			data := getData()

			if data!=nil {
				paint(data, buf)
				bufCopy := *buf
				bufPipe <- &bufCopy
			}

			select {
			case <-tick.C:
				continue
			case <-shutdown:
				return
			}
		}
	}(bufPipe, shutdown)

	if err := ebiten.Run(update, scrWidth, scrHeight, 1.0, "Tempomat Show"); err != nil {
		log.Fatal(err)
	}

	shutdown <- true
}

func isNice(l, a, b float64) bool {
	h, c, L := colorful.LabToHcl(l, a, b)
	return 150.0 < h && h < 250.0 && 0.2 < c && c < 0.8 && 0.0 < L && L < 0.8
}

func getData() *api.DumpList {
	slash32 := api.DumpList{}
	args := api.DumpArgs{
		BucketName: "Slash32",
	}
	err := tempomatClient.Call("TempomatAPI.Dump", &args, &slash32)
	if err != nil {
		log.Printf("Call error: %s", err)
		return nil
	}

	sort.Sort(api.TitleSortDumpList(slash32))
	return &slash32
}

func paint(slash32 *api.DumpList, buf *image.RGBA) {
	total := 0.0
	for _, e := range *slash32 {
		if time.Since(e.LastUsed)>10*time.Second {
			continue
		}

		// rps := 1.0/e.AvgSincePrev.Seconds()
		total += e.AvgCpuSecs

		if _, found := palMap[e.Title]; !found {
			palMap[e.Title] = palette[palPointer]

			palPointer++
			if palPointer>=len(palette) {
				// Rotate palette.
				palPointer = 0
			}
		}
	}

	moveLeft(buf)
	curY := 0
	for _, e := range *slash32 {
		if time.Since(e.LastUsed)>10*time.Second {
			continue
		}

		length := int((e.AvgCpuSecs/total)*float64(scrHeight))
		nextY := curY+length
		for y := curY; y<nextY; y++ {
			buf.Set(scrWidth-1, y, palMap[e.Title])
		}
		curY = nextY
	}
}

func moveLeft(buf *image.RGBA) {
	b := buf.Bounds()
	t := image.Pt(1, 0)
	draw.Draw(buf, b, buf, b.Min.Add(t), draw.Src)
}

func update(screen *ebiten.Image) error {
	if ebiten.IsRunningSlowly() {
		return nil
	}

	select {
	case buf := <-bufPipe:
		backBuf = buf
	default:
	}

	screen.ReplacePixels(backBuf.Pix)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %f", ebiten.CurrentFPS()))
	return nil
}
