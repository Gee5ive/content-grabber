package ContentGrabber

import (
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-multierror"
	"github.com/parnurzeal/gorequest"
	"strings"
	"time"
)

type ImagesResponse struct {
	Images []string
	Err    error
}

type ImageSource func(keyWord, proxy string, pageNumber int) ImagesResponse
type pageFunc func(pageNumber int, keyWord string) string
type finderFunc func(doc *goquery.Document) []string

func client(proxy string) *gorequest.SuperAgent {
	if proxy != "" {
		return gorequest.New().Timeout(time.Second * 2).Proxy(proxy)
	}
	return gorequest.New()

}
func ImageSourceFactory(pageFunc pageFunc, finderFunc finderFunc) ImageSource {
	source := func(keyWord, proxy string, pageNumber int) ImagesResponse {
		find := func() ImagesResponse {
			distinct := func(inSlice []string) []string {
				keys := make(map[string]bool)
				var list []string
				for _, entry := range inSlice {
					if _, value := keys[entry]; !value {
						keys[entry] = true
						list = append(list, entry)
					}
				}
				return list
			}
			req := client(proxy)
			var resErr error
			resp, _, err := req.Get(pageFunc(pageNumber, keyWord)).End()
			if err != nil {
				for _, e := range err {
					if e != nil {
						resErr = multierror.Append(resErr, e)
					}

				}
				return ImagesResponse{Err: resErr}
			}
			if resp == nil {
				return ImagesResponse{Err: errors.New("invalid or empty response")}
			}
			defer resp.Body.Close()

			doc, gErr := goquery.NewDocumentFromReader(resp.Body)
			if gErr != nil {
				return ImagesResponse{Err: gErr}
			}
			images := distinct(finderFunc(doc))
			if images == nil {
				return ImagesResponse{Err: errors.New("no images found")}
			}
			return ImagesResponse{Images: images, Err: nil}

		}
		return find()
	}
	return source
}

func PixaBay() ImageSource {

	page := func(pageNumber int, keyWord string) string {
		return fmt.Sprintf("https://pixabay.com/images/search/%s/?pagi=%d", keyWord, pageNumber)
	}
	find := func(doc *goquery.Document) []string {
		var images []string
		doc.Find(".item").Each(func(i int, s *goquery.Selection) {
			s.Find("a").Each(func(i int, selection *goquery.Selection) {
				img, ok := selection.Find("img").Attr("src")
				if ok {
					if img != "/static/img/blank.gif" {
						images = append(images, img)
					}

				}
			})

		})
		return images
	}
	return ImageSourceFactory(page, find)
}

func ShutterStock() ImageSource {
	page := func(pageNumber int, keyWord string) string {
		return fmt.Sprintf("https://www.shutterstock.com/search/%s?src=aw.ds&orientation=horizontal&page=%d", keyWord, pageNumber)
	}
	find := func(doc *goquery.Document) []string {
		var images []string
		doc.Find(".z_e_h").Each(func(i int, s *goquery.Selection) {

			img, ok := s.Attr("src")
			if ok && img != "" {
				images = append(images, img)
			}
		})
		return images
	}
	return ImageSourceFactory(page, find)
}

func Burst() ImageSource {
	page := func(pageNumber int, keyWord string) string {
		return fmt.Sprintf("https://burst.shopify.com/photos/search?page=%d&q=%s&utf8=%%E2%%9C%%93", pageNumber, keyWord)
	}
	find := func(doc *goquery.Document) []string {
		var images []string
		fileTypes := []string{".png", ".jpg"}
		doc.Find(".tile__image").Each(func(i int, s *goquery.Selection) {
			img, ok := s.Attr("src")
			if ok && img != "" {
				for _, fileType := range fileTypes {
					if strings.Contains(img, fileType) {
						images = append(images, strings.SplitAfter(img, fileType)[0])
					}
				}

			}
		})

		return images
	}
	return ImageSourceFactory(page, find)
}

func PicJumBo() ImageSource {
	page := func(pageNumber int, keyWord string) string {
		return fmt.Sprintf("https://picjumbo.com/page/%d/?s=%s", pageNumber, keyWord)
	}
	find := func(doc *goquery.Document) []string {
		var images []string
		doc.Find(".image").Each(func(i int, s *goquery.Selection) {
			img, ok := s.Attr("src")
			if ok && img != "" {
				images = append(images, img)

			}
		})
		return images
	}
	return ImageSourceFactory(page, find)
}

func IsoRepublic() ImageSource {
	page := func(pageNumber int, keyWord string) string {
		return fmt.Sprintf("https://isorepublic.com/page/%d/?s=%s&post_type=photo_post", pageNumber, keyWord)
	}
	find := func(doc *goquery.Document) []string {
		var images []string
		doc.Find(".photo-grid-item").Each(func(i int, s *goquery.Selection) {
			img, ok := s.Find("img").Attr("src")
			if ok && img != "" {
				images = append(images, img)

			}
		})
		return images
	}
	return ImageSourceFactory(page, find)
}

func AllImageSources() []ImageSource {
	return []ImageSource{IsoRepublic(), PicJumBo(), Burst(), ShutterStock(), PixaBay()}
}
