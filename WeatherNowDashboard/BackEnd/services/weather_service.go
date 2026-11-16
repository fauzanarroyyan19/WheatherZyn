package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"weathernow/models"
)

type apiResponse struct {
	Name string `json:"name"`
	Main struct {
		Temp     float64 `json:"temp"`
		Humidity int     `json:"humidity"`
	} `json:"main"`
	Weather []struct {
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
}

func GetWeather(city string) (*models.WeatherResponse, error) {
	apiKey := os.Getenv("OPENWEATHER_API_KEY")
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric&lang=id", city, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("gagal ambil data dari API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API balas status %d", resp.StatusCode)
	}

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("gagal decode data JSON: %v", err)
	}

	if len(data.Weather) == 0 {
		return nil, fmt.Errorf("data cuaca tidak ditemukan untuk kota %s", city)
	}

	return &models.WeatherResponse{
		City:        strings.Title(data.Name),
		Temperature: data.Main.Temp,
		Humidity:    data.Main.Humidity,
		Description: strings.Title(data.Weather[0].Description),
		Icon:        fmt.Sprintf("https://openweathermap.org/img/wn/%s.png", data.Weather[0].Icon),
	}, nil
}
