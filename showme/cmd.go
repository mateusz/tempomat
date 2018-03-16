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
	"sync"
	"github.com/lucasb-eyer/go-colorful"
	"golang.org/x/image/draw"
)

const (
	tickPeriod = 1000 * time.Millisecond
)

var (
	bufMutex sync.RWMutex
	buf *image.RGBA
	palette []colorful.Color
	palPointer int
	palMap map[string]colorful.Color
	scrWidth int
	scrHeight int
	tempomatClient *rpc.Client
)

func main() {
	scrWidth = 1000
	scrHeight = 500
	buf = image.NewRGBA(image.Rect(0, 0, scrWidth, scrHeight))

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
	go func(shutdown chan bool) {
		tick := time.NewTicker(tickPeriod)
		defer tick.Stop()
		for {
			getData()

			select {
			case <-tick.C:
				continue
			case <-shutdown:
				return
			}
		}
	}(shutdown)

	if err := ebiten.Run(update, scrWidth, scrHeight, 1.0, "Tempomat Show"); err != nil {
		log.Fatal(err)
	}

	shutdown <- true
}

func isNice(l, a, b float64) bool {
	h, c, L := colorful.LabToHcl(l, a, b)
	return 100.0 < h && h < 200.0 && 0.4 < c && c < 0.8 && 0.4 < L && L < 0.8
}

func getData() {
	slash32 := api.DumpList{}
	args := api.DumpArgs{
		BucketName: "Slash32",
	}
	err := tempomatClient.Call("TempomatAPI.Dump", &args, &slash32)
	if err != nil {
		log.Printf("Call error: %s", err)
		return
	}

	sort.Sort(api.TitleSortDumpList(slash32))

	total := 0.0
	for _, e := range slash32 {
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

	bufMutex.Lock()
	moveLeft()
	curY := 0
	for _, e := range slash32 {
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
	bufMutex.Unlock()
}

func moveLeft() {
	b := buf.Bounds()
	t := image.Pt(1, 0)
	draw.Draw(buf, b, buf, b.Min.Add(t), draw.Src)
}

func update(screen *ebiten.Image) error {
	if ebiten.IsRunningSlowly() {
		return nil
	}

	bufMutex.RLock()
	screen.ReplacePixels(buf.Pix)
	bufMutex.RUnlock()
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %f", ebiten.CurrentFPS()))
	return nil
}
