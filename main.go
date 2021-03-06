package main

import (
        "bufio"
        "flag"
        "fmt"
        "net/http"
        "net/url"
        "os"
        "strings"
	"strconv"

        "github.com/PuerkitoBio/goquery"
	"github.com/logrusorgru/aurora"
)

type logger interface {
        Log(msg string)
	Error(msg string, err error)
}

type silent struct{}

func (s silent) Log(msg string) {
}

func (s silent) Error(msg string, err error) {
}

type verbose struct{
	
}

func (v verbose) Log(msg string) {
	fmt.Println(au.Cyan(msg))
}

func Log(l logger, msg string) {
        l.Log(msg)
}

func (v verbose) Error(msg string, err error) {	
	fmt.Fprintln(os.Stderr, au.Red(msg))
	if err != nil {
		fmt.Fprintln(os.Stderr, au.Red("[!] Error: "), au.Red(err))
	}
}

func Error(l logger, msg string, err error) {
	l.Error(msg, err)
}

var output logger
var au aurora.Aurora

func main() {
	urlArg := flag.String("url", "", "The url to get the javascript sources from")
	outputFileArg := flag.String("output", "", "Output file to save the results to")
	inputFileArg := flag.String("input", "", "Input file with urls")
	resolveArg := flag.Bool("resolve", false, "Output only existing files")
	completeArg := flag.Bool("complete", false, "Complete the url. e.g. append the domain to the path")
	plainArg := flag.Bool("plain", false, "Output only the results")
	silentArg := flag.Bool("silent", false, "Dont output anything")
	noColorsArg := flag.Bool("nocolors", false, "Enable or disable colors")
	flag.Parse()

	au = aurora.NewAurora(!*noColorsArg)

        var urls []string
        var allSources []string

        if *silentArg || *plainArg {
                output = silent{}
        } else {
                output = verbose{}
        }

        stat, err := os.Stdin.Stat()
        if err != nil {
                output.Error("[!] Couldnt read Stdin", err)
        }

        if (stat.Mode() & os.ModeCharDevice) == 0 {
                scanner := bufio.NewScanner(os.Stdin)
                for scanner.Scan() {
                        urls = append(urls, scanner.Text())
                }
                if err := scanner.Err(); err != nil {
                        output.Error("[!] Couldnt read Stdin", err)
                }
                if len(urls) > 0 {
                        output.Log("[+] Received urls from Stdin")
                }
        }

        if *inputFileArg != "" {
                lines, err := readLines(*inputFileArg)
                if err != nil {
                        output.Error("[!] Couldn't read from input file", err)
                }
                output.Log("[+] Set url file to " + *inputFileArg)
                urls = append(urls, lines...)
        }

        if *urlArg != "" {
                output.Log("[+] Set url to " + *urlArg)
                urls = append(urls, *urlArg)
        }

        if len(urls) == 0 {
                output.Error("[!] No urls supplied", nil)
                os.Exit(3)
        }

        if *resolveArg && !*completeArg {
                output.Error("[!] Resolve can only be used in combination with -complete", nil)
                os.Exit(3)
        }

        for _, e := range urls {
                var sourcesBak []string
                var completedSuccessfully = true
                output.Log("[+] Getting sources from " + e)
                sources, err := getScriptSrc(e)
                if err != nil {
                        output.Error("[!] Couldn't get sources from " + e, err)
                }

                if *completeArg {
                        output.Log("[+] Completing URLs")
                        sourcesBak = sources
                        sources, err = completeUrls(sources, e)
                        if err != nil {
                                output.Error("[!] Couldn't complete URLs", err)
                                sources = sourcesBak
                                completedSuccessfully = false
                        }
                }

                if *resolveArg && *completeArg {
                        if completedSuccessfully {
                                output.Log("[+] Resolving files")
                                sourcesBak = sources
                                sources, err = resolveUrls(sources)
                                if err != nil {
                                        output.Error("[!] Couldn't resolve URLs", err)
                                        sources = sourcesBak
                                }
                        } else {
                                output.Error("[!] Couldn't resolve URLs", nil)
                        }
                } else if *resolveArg {
                        output.Error("[!] Resolve can only be used in combination with -complete", nil)
                }

                if !*silentArg {
                        for _, i := range sources {
                                fmt.Println(i)
                        }
                }

                if *outputFileArg != "" {
                        allSources = append(allSources, sources...)
                }

        }

        // Save to file
        if *outputFileArg != "" {
                output.Log("[+] Saving output to " + *outputFileArg)
                err := saveToFile(allSources, *outputFileArg)
                if err != nil {
                        output.Error("[!] Couldn't save to output file " + *outputFileArg, err)
                }
        }

}

// ToDO: Use channel instead of slide, and use io.Writer instead of file path
func saveToFile(sources []string, path string) error {
        file, err := os.Create(path)
        if err != nil {
                return err
        }
        defer file.Close()

        w := bufio.NewWriter(file)
        for _, line := range sources {
                fmt.Fprintln(w, line)
        }
        return w.Flush()
}

func getScriptSrc(url string) ([]string, error) {
        // Request the HTML page.
        res, err := http.Get(url)
        if err != nil {
                return nil, err
        }
        defer res.Body.Close()
        if res.StatusCode != 200 {
                output.Error("[!] " + url + " returned an " + strconv.Itoa(res.StatusCode) + " instead of an 200 OK", nil)
                return nil, nil
        }

        // Load the HTML document
        doc, err := goquery.NewDocumentFromReader(res.Body)
        if err != nil {
                return nil, err
        }

        var sources []string

        // Find the script tags, and get the src
        doc.Find("script").Each(func(i int, s *goquery.Selection) {
                src, _ := s.Attr("src")
                if src != "" {
                        sources = append(sources, src)
                }
        })

        return sources, nil
}

// ToDo: Use io.Writer instead of a file path
func readLines(path string) ([]string, error) {
        file, err := os.Open(path)
        if err != nil {
                return nil, err
        }
        defer file.Close()

        var lines []string
        scanner := bufio.NewScanner(file)
        for scanner.Scan() {
                lines = append(lines, scanner.Text())
        }
        return lines, scanner.Err()
}

func resolveUrls(s []string) ([]string, error) {
        for i := len(s) - 1; i >= 0; i-- {
                resp, err := http.Get(s[i])
                if err != nil {
                        return nil, err
                }
                if resp.StatusCode != 200 && resp.StatusCode != 304 {
                        s = append(s[:i], s[i+1:]...)
                }
        }
        return s, nil
}

func completeUrls(s []string, mainUrl string) ([]string, error) {
        u, err := url.Parse(mainUrl)
        if err != nil {
                return nil, err
        }

        for i, _ := range s {
                if strings.HasPrefix(s[i], "//") {
                        s[i] = u.Scheme + ":" + s[i]
                } else if strings.HasPrefix(s[i], "/") && string(s[i][1]) != "/" {
                        s[i] = u.Scheme + "://" + u.Host + s[i]
                } else if !strings.HasPrefix(s[i], "http://") && !strings.HasPrefix(s[i], "https://") {
                        s[i] = u.Scheme + "://" + u.Host + u.Path + "/" + s[i]
                }
        }
        return s, nil
}
