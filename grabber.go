package ContentGrabber

import (
	ctx "context"
	"errors"
	"fmt"
	"github.com/gammazero/workerpool"
	"github.com/hashicorp/go-multierror"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
)

func sleep(delay RequestDelay) {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)
	sleepTime := r.Intn(delay.Max-delay.Min) + delay.Min
	time.Sleep(time.Millisecond * time.Duration(sleepTime))
}

type RequestDelay struct {
	Max int
	Min int
}
type ErrHandler interface {
	SendErr(err error)
}
type DownloadResponse struct {
	FilePath string
	Err      error
}

type ImageGrabber struct {
	pool       *workerpool.WorkerPool
	reqDelay   RequestDelay
	pageDepth  int
	sources    []ImageSource
	keyWords   []string
	errHandler ErrHandler
	proxies    []string
	saveDIR    string
	ctx        ctx.Context
	done       ctx.CancelFunc
}

func NewImageGrabber(maxConn int, pageDepth int, keyWords, proxies []string, sources []ImageSource, saveDIR string, delay RequestDelay, handler ErrHandler) ImageGrabber {
	pool := workerpool.New(maxConn)
	grabCtx, cancel := ctx.WithCancel(ctx.Background())
	return ImageGrabber{pool: pool, keyWords: keyWords, pageDepth: pageDepth,
		sources: sources, errHandler: handler, proxies: proxies, saveDIR: saveDIR, ctx: grabCtx, done: cancel, reqDelay: delay}
}

func (g ImageGrabber) delay() {
	sleep(g.reqDelay)
}
func (g ImageGrabber) Grab() []string {
	startTime := time.Now()
	log.Print("image grabber started")
	proxy := func() string {
		source := rand.NewSource(time.Now().UnixNano())
		r := rand.New(source)
		return g.proxies[r.Intn(len(g.proxies))]
	}

	var imageURLChanList []<-chan ImagesResponse

	for _, source := range g.sources {
		for _, keyword := range g.keyWords {
			for i := 0; i < g.pageDepth; i++ {
				g.delay()
				log.Printf("imageURL scrape worker %d starterd", i)
				worker := func() {
					res := source(g.ctx, keyword, proxy(), i)
					imageURLChanList = append(imageURLChanList, res)
				}
				g.pool.Submit(worker)
			}
		}
	}

	var urls []string
	for _, channels := range imageURLChanList {
		for resp := range channels {
			if resp.Err != nil {
				g.errHandler.SendErr(resp.Err)
				continue
			}
			for _, url := range resp.Images {
				if url != "" {
					urls = append(urls, url)
				}
			}
		}
	}

	log.Print("finished parsing urls")
	var downLoadChanList []<-chan DownloadResponse
	log.Printf("image dowloading begun")
	for i, url := range urls {
		worker := func() {
			proxy := proxy()
			log.Print(fmt.Sprintf("download of image %s begun using proxy : %s download worker # %d", url, proxy, i))
			res := downLoader(g.ctx, url, g.saveDIR, proxy)
			downLoadChanList = append(downLoadChanList, res)
		}
		g.delay()
		g.pool.Submit(worker)
	}
	var images []string
	for _, fileChan := range downLoadChanList {
		for resp := range fileChan {
			if resp.Err != nil {
				g.errHandler.SendErr(resp.Err)
				continue
			}
			if resp.FilePath != "" {
				images = append(images, resp.FilePath)
			}

		}
	}
	endTime := time.Since(startTime)
	log.Printf("image grabber job complete, took : %v", endTime)
	return images
}

func (g ImageGrabber) Stop() {
	g.done()
}
func downLoader(ctx ctx.Context, fileURL, saveDir, proxy string) <-chan DownloadResponse {
	getFilePath := func(file string) string {
		parts := strings.Split(fileURL, "/")
		return fmt.Sprintf("%s%s%s", saveDir, string(os.PathSeparator), parts[len(parts)-1])
	}
	filePath := getFilePath(fileURL)
	downLoad := func() DownloadResponse {
		req := client(proxy)
		var resErr error
		resp, _, err := req.Get(fileURL).End()
		if err != nil {
			for _, e := range err {
				if e != nil {
					resErr = multierror.Append(resErr, e)
				}
			}
			return DownloadResponse{Err: resErr}
		}
		if resp == nil {
			return DownloadResponse{Err: errors.New("invalid file url")}
		}

		defer resp.Body.Close()

		if _, dirErr := os.Stat(saveDir); os.IsNotExist(dirErr) {
			if cErr := os.Mkdir(saveDir, os.ModePerm); cErr != nil {
				return DownloadResponse{Err: cErr}
			}
		}
		f, oErr := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
		if oErr != nil {
			return DownloadResponse{Err: oErr}
		}
		if f != nil {
			defer f.Close()

		}
		if _, lErr := io.Copy(f, resp.Body); lErr != nil {
			return DownloadResponse{Err: lErr}
		}
		log.Printf("file %s downloded worker complete", fileURL)
		return DownloadResponse{FilePath: filePath, Err: nil}

	}
	respChan := make(chan DownloadResponse)
	go func() {
		defer close(respChan)
		select {
		case <-ctx.Done():
			respChan <- DownloadResponse{Err: ctx.Err()}
			return
		case respChan <- downLoad():

		}
	}()
	return respChan
}
