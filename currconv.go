package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

var apiKey string
var data Data

// cache templates for later use
var templates = template.Must(template.ParseFiles("index.html", "convert.html", "contact.html", "about.html"))

// reads and returns api key stored in filename
func readAPIKey(filename string) string {
	var data, err = ioutil.ReadFile("key.txt")
	if err != nil {
		fmt.Println("File reading error", err)
		log.Fatal(err)
	}
	return string(data)
}

// Data stores data from api request for re-use
type Data struct {
	Success bool
	// timestamp of api request
	Timestamp int64
	// base currency for calculations
	Base string
	Date string
	// maps currency identifiers to their value in base currency
	Rates map[string]float64
}

// calculates how much "amount" of curr1 is worth in curr2
func (data Data) convert(curr1 string, curr2 string, amount float64) float64 {
	euroAmount := amount / data.Rates[curr1]
	return euroAmount * data.Rates[curr2]
}

// returns data if less than 1 hour has passed since data.Timestamp
// returns newly fetched API data otherwise
func (data Data) update() Data {
	timestamp := time.Unix(data.Timestamp, 0)
	timePassed := time.Since(timestamp)
	if timePassed.Hours() > 1 {
		// only update if data is older than 1 hour to limit API requests made
		log.Println("Data is older than 1 hour, refreshing")
		b := getData()
		log.Println(string(b))
		d := decodeJSON(b)
		return d
	}
	return data
}

// Page stores variables for /convert/
type Page struct {
	From   string
	To     string
	Value  float64
	Result float64
	Time   string
}

// sends an API request to fixer to get currency conversion data
// returns string containing json
func getData() []byte {
	resp, err := http.Get("http://data.fixer.io/api/latest?access_key=" + apiKey)
	if err != nil {
		log.Println(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Println(err)
	}

	return body
}

// takes json as returned by getData() and creates Data struct with corresponding values
func decodeJSON(b []byte) Data {
	// can't decode directly from JSON to Data struct because of map[string]float64 field
	var i interface{}

	err := json.Unmarshal(b, &i)

	if err != nil {
		log.Fatalln(err)
	}

	m := i.(map[string]interface{})

	success := m["success"].(bool)
	timestamp := int64(m["timestamp"].(float64))
	base := m["base"].(string)
	date := m["date"].(string)
	ratesInterface := m["rates"].(map[string]interface{})

	rates := make(map[string]float64)

	for key, value := range ratesInterface {
		rates[key] = value.(float64)
	}

	data := Data{success, timestamp, base, date, rates}

	return data
}

// rounds float to 2 places after decimal point
func roundTo2Decimals(x float64) float64 {
	return (math.Round(x*100) / 100)
}

// executes template tmpl.html using ResponseWriter w
func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// generates a generic handler function that renders a template
func makeGenericHandler(tmpl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, tmpl, nil)
	}
}

// extracts variables from url query and uses them for currency conversion calculation
// renders convert template
func convertHandler(w http.ResponseWriter, r *http.Request) {
	data = data.update()

	from := r.URL.Query()["from"][0]
	to := r.URL.Query()["to"][0]
	value, err := strconv.ParseFloat(r.URL.Query()["value"][0], 8)

	// check if conversion rates are available for both currencies
	_, okFrom := data.Rates[from]
	_, okTo := data.Rates[to]
	if err != nil || !okFrom || !okTo {
		// redirect if float entered was invalid or the chosen currencies are unavailable (only happens if URL is modified manually)
		http.Redirect(w, r, "/", 302)
		return
	}

	time := fmt.Sprint(time.Unix(data.Timestamp, 0))

	result := data.convert(from, to, value)
	result = roundTo2Decimals(result)

	p := Page{from, to, value, result, time}

	renderTemplate(w, "convert", &p)
}

// evaluates form data and redirects to /convert/ page with corresponding url parameters
func redirectHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	from := r.Form["from"][0]
	to := r.Form["to"][0]
	value := r.Form["value"][0]

	url := "/convert/?from=" + from + "&to=" + to + "&value=" + value

	http.Redirect(w, r, url, 302)
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "8080"
	}
	return port
}

func main() {
	apiKey = readAPIKey("key.txt")

	b := getData()
	data = decodeJSON(b)

	http.HandleFunc("/", makeGenericHandler("index"))
	http.HandleFunc("/convert/", convertHandler)
	http.HandleFunc("/redirect/", redirectHandler)
	http.HandleFunc("/about/", makeGenericHandler("about"))
	http.HandleFunc("/contact/", makeGenericHandler("contact"))

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
