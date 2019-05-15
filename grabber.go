package ContentGrabber

import (
	ctx "context"
	"errors"
	"fmt"
	"github.com/gammazero/workerpool"
	"github.com/hashicorp/go-multierror"
	"io"
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

// Logger is a minimal interface for Logger instances
type Logger interface {
	Info(msg string, fields ...map[string]interface{})
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
	reqDelay   RequestDelay
	pageDepth  int
	sources    []ImageSource
	keyWords   []string
	errHandler ErrHandler
	proxies    []string
	saveDIR    string
	maxConn    int
	ctx        ctx.Context
	done       ctx.CancelFunc
	logger     Logger
}

func NewImageGrabber(maxConn int, pageDepth int, keyWords, proxies []string, sources []ImageSource, saveDIR string, delay RequestDelay, handler ErrHandler, logger ...Logger) ImageGrabber {
	grabCtx, cancel := ctx.WithCancel(ctx.Background())
	grabber := ImageGrabber{maxConn: maxConn, keyWords: keyWords, pageDepth: pageDepth,
		sources: sources, errHandler: handler, proxies: proxies, saveDIR: saveDIR, ctx: grabCtx, done: cancel, reqDelay: delay}
	if len(logger) > 0 {
		grabber.logger = logger[0]
	}
	return grabber
}

func (g ImageGrabber) pool() *workerpool.WorkerPool {
	return workerpool.New(g.maxConn)
}

func (g ImageGrabber) delay() {
	sleep(g.reqDelay)
}

func (g ImageGrabber) proxy() string {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)
	return g.proxies[r.Intn(len(g.proxies))]
}

func (g ImageGrabber) log(msg string, fields ...map[string]interface{}) {
	if g.logger != nil {
		g.logger.Info(msg, fields...)
	}
}

func (g ImageGrabber) grabURLS() []string {
	pool := g.pool()
	g.log("image grabber started")
	var imageURLs []ImagesResponse
	for _, source := range g.sources {
		for _, keyword := range g.keyWords {
			for i := 0; i < g.pageDepth; i++ {
				g.delay()
				g.log(fmt.Sprintf("imageURL grabber for keyword : %s worker %d starterd", keyword, i))
				worker := func() {
					imageURLs = append(imageURLs, source(keyword, g.proxy(), i))
				}
				pool.Submit(worker)
			}
		}
	}
	pool.StopWait()

	var urls []string
	for _, resp := range imageURLs {

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
	g.log("url scraping complete")
	return urls

}

func (g ImageGrabber) Grab() []string {
	pool := g.pool()
	urls := g.grabURLS()
	startTime := time.Now()
	var downLoadList []DownloadResponse
	g.log("image dowloading begun")
	for i, url := range urls {
		worker := func() {
			proxy := g.proxy()
			g.log(fmt.Sprintf("download of image %s begun using proxy : %s download worker # %d", url, proxy, i))
			res := g.downLoader(url, g.saveDIR, proxy)
			downLoadList = append(downLoadList, res)
		}
		g.delay()
		pool.Submit(worker)
	}
	pool.StopWait()
	var images []string
	for _, resp := range downLoadList {
		if resp.Err != nil {
			g.errHandler.SendErr(resp.Err)
			continue
		}
		if resp.FilePath != "" {
			images = append(images, resp.FilePath)
		}

	}
	endTime := time.Since(startTime)
	g.log(fmt.Sprintf("image grabber job complete, took : %v", endTime))
	return images
}

func (g ImageGrabber) Stop() {
	g.done()
}
func (g ImageGrabber) downLoader(fileURL, saveDir, proxy string) DownloadResponse {
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
		g.log(fmt.Sprintf("file %s downloded worker complete", fileURL))
		return DownloadResponse{FilePath: filePath, Err: nil}

	}

	return downLoad()
}
