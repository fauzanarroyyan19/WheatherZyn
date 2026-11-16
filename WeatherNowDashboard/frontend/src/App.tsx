import { useEffect, useMemo, useState } from 'react'

type Language = 'id' | 'en'
type Hour = { time: number; temperature: number; description: string; icon: string; rain: number }
type Day = { date: string; min: number; max: number; description: string; icon: string; rain: number }
type Alert = { event: string; description: string; start: number; end: number; source: string }
type Weather = { city: string; temperature: number; feels_like: number; status: string; description: string; humidity: number; wind_speed: number; icon: string; timezone: number; sunrise: number; sunset: number; uv: number | null; hourly: Hour[]; forecast: Day[]; alerts: Alert[]; official_alerts: Alert[]; forecast_source: string }
type History = { city: string; time: number; temperature: number }

const starterCities = ['Jakarta', 'Bandung', 'Bekasi', 'Surabaya', 'Denpasar', 'Makassar']
const API = import.meta.env.VITE_WEATHER_API_URL || 'http://localhost:8080'
const copy = {
  id: {
    tagline: 'TEMAN CUACA HARIAN ANDA', title1: 'Cuaca hari ini,', title2: 'lebih dekat.', intro: 'Pantau kondisi langit, lihat prakiraan, dan rencanakan harimu dengan lebih nyaman.', live: 'Cuaca terkini', location: 'LOKASI', localTime: 'WAKTU SETEMPAT', loading: 'Memperbarui cuaca…', feels: 'Terasa seperti', humidity: 'KELEMBAPAN', wind: 'KECEPATAN ANGIN', uv: 'INDEKS UV', oneCall: 'One Call', sunrise: 'Terbit', sunset: 'Terbenam', saveCity: 'Simpan kota', favorite: 'Favorit', quickCities: 'KOTA CEPAT', favoriteCities: 'KOTA FAVORIT', addCity: 'Tambah kota…', removeFavorite: (city: string) => `Hapus ${city} dari favorit`, saveFavorite: 'Simpan kota ke favorit', errorTitle: 'Cuaca belum dapat dimuat', retry: 'Coba lagi', forecast: 'PRAKIRAAN', comingWeather: 'Cuaca mendatang', days5: '5 hari', hourly: 'Per jam', every3Hours: '3 jam', dataEvery3Hours: 'Data gratis OpenWeather tersedia setiap 3 jam.', dataHourly: 'Prakiraan per jam dari OpenWeather One Call.', historyLabel: 'PANTAU PERUBAHAN', historyTitle: 'Riwayat suhu', records: 'catatan', historyEmpty: 'Grafik akan terbentuk setelah cuaca kota ini dimuat beberapa kali.', attention: 'Perhatian cuaca', rainChance: 'Peluang hujan tinggi', footer: 'Data cuaca oleh OpenWeather', madeSimple: 'Cuaca, lebih mudah.', invalidCity: 'Kota tidak ditemukan. Periksa ejaan nama kota lalu coba lagi.', backendMissing: 'Pastikan backend cuaca sedang berjalan.', langToggle: 'Switch to English', dayLocale: 'id-ID', conditions: 'KONDISI'
  },
  en: {
    tagline: 'YOUR DAILY WEATHER COMPANION', title1: 'Today’s weather,', title2: 'closer to you.', intro: 'Check the sky, explore the forecast, and plan your day with confidence.', live: 'Live weather', location: 'LOCATION', localTime: 'LOCAL TIME', loading: 'Updating weather…', feels: 'Feels like', humidity: 'HUMIDITY', wind: 'WIND SPEED', uv: 'UV INDEX', oneCall: 'One Call', sunrise: 'Sunrise', sunset: 'Sunset', saveCity: 'Save city', favorite: 'Favorite', quickCities: 'QUICK CITIES', favoriteCities: 'FAVORITE CITIES', addCity: 'Add a city…', removeFavorite: (city: string) => `Remove ${city} from favorites`, saveFavorite: 'Save city to favorites', errorTitle: 'Weather could not be loaded', retry: 'Try again', forecast: 'FORECAST', comingWeather: 'Upcoming weather', days5: '5 days', hourly: 'Hourly', every3Hours: '3 hours', dataEvery3Hours: 'OpenWeather free forecast data is provided every 3 hours.', dataHourly: 'Hourly forecast from OpenWeather One Call.', historyLabel: 'TRACK CHANGES', historyTitle: 'Temperature history', records: 'records', historyEmpty: 'The chart will appear after this city’s weather has been loaded a few times.', attention: 'Weather alerts', rainChance: 'High chance of rain', footer: 'Weather data by OpenWeather', madeSimple: 'Weather, made simple.', invalidCity: 'City not found. Check the spelling and try again.', backendMissing: 'Make sure the weather backend is running.', langToggle: 'Ganti ke Bahasa Indonesia', dayLocale: 'en-US', conditions: 'CONDITIONS'
  }
}

function localTime(unix: number, offset: number, language: Language, options: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }) {
  return new Intl.DateTimeFormat(language === 'id' ? 'id-ID' : 'en-US', { ...options, timeZone: 'UTC' }).format(new Date((unix + offset) * 1000))
}

function App() {
  const [language, setLanguage] = useState<Language>(() => localStorage.getItem('weathernow-language') === 'en' ? 'en' : 'id')
  const [city, setCity] = useState('Jakarta')
  const [weather, setWeather] = useState<Weather | null>(null)
  const [history, setHistory] = useState<History[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [favorites, setFavorites] = useState<string[]>(() => { try { return JSON.parse(localStorage.getItem('weathernow-favorites') || '[]') } catch { return [] } })
  const [newCity, setNewCity] = useState('')
  const [forecastMode, setForecastMode] = useState<'daily' | 'hourly'>('daily')
  const t = copy[language]

  useEffect(() => {
    localStorage.setItem('weathernow-language', language)
    document.documentElement.lang = language
    document.title = language === 'id' ? 'WeatherNow — Cuaca' : 'WeatherNow — Weather'
  }, [language])

  useEffect(() => {
    let active = true
    setLoading(true); setError('')
    fetch(`${API}/api/weather?city=${encodeURIComponent(city)}&lang=${language}`)
      .then(async response => {
        const data = await response.json()
        if (!response.ok) throw new Error(data.error || t.backendMissing)
        return data as Weather
      })
      .then(data => {
        if (!active) return
        setWeather(data)
        fetch(`${API}/api/history?city=${encodeURIComponent(city)}`).then(res => res.ok ? res.json() : []).then(points => { if (active) setHistory(points as History[]) }).catch(() => { if (active) setHistory([]) })
      })
      .catch(err => { if (active) setError(err instanceof Error ? err.message : t.backendMissing) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [city, language, reload])

  useEffect(() => { localStorage.setItem('weathernow-favorites', JSON.stringify(favorites)) }, [favorites])
  const cityOptions = useMemo(() => [...new Set([...starterCities, ...favorites, city])], [favorites, city])
  const allAlerts = [...(weather?.official_alerts || []), ...(weather?.alerts || [])]
  const temps = history.map(point => point.temperature)
  const minTemp = temps.length ? Math.min(...temps) : 0
  const maxTemp = temps.length ? Math.max(...temps) : 1
  const range = Math.max(maxTemp - minTemp, 1)
  const chartPoints = history.map((point, index) => `${history.length === 1 ? 300 : index * 600 / (history.length - 1)},${104 - ((point.temperature - minTemp) / range) * 84}`).join(' ')
  const favoriteCurrent = favorites.some(item => item.toLowerCase() === city.toLowerCase())

  const addFavorite = () => {
    const value = newCity.trim()
    if (value && !favorites.some(item => item.toLowerCase() === value.toLowerCase())) setFavorites([...favorites, value])
    if (value) { setCity(value); setNewCity('') }
  }
  const toggleFavorite = () => setFavorites(favoriteCurrent ? favorites.filter(item => item.toLowerCase() !== city.toLowerCase()) : [...favorites, city])

  return <main className="weather-page">
    <div className="ambient ambient-one"/><div className="ambient ambient-two"/>
    <header className="topbar">
      <a className="brand" href="#"><span className="brand-mark">☼</span> weather<span>now</span></a>
      <div className="header-actions"><span className="live"><i/> {t.live}</span><button className="lang-toggle" onClick={() => setLanguage(language === 'id' ? 'en' : 'id')} aria-label={t.langToggle}>{language === 'id' ? 'EN' : 'ID'}</button></div>
    </header>
    <section className="dashboard">
      <div className="intro"><p className="eyebrow">{t.tagline}</p><h1>{t.title1}<br/><em>{t.title2}</em></h1><p className="intro-copy">{t.intro}</p></div>
      <div className="weather-panel">
        <div className="panel-top"><div><span className="label">{t.location}</span><label className="city-picker">⌖ <select value={city} onChange={e => setCity(e.target.value)}>{cityOptions.map(item => <option key={item}>{item}</option>)}</select><span className="chevron">⌄</span></label></div><div className="local-time"><span className="label">{t.localTime}</span><strong>{weather ? localTime(Math.floor(Date.now()/1000), weather.timezone, language) : '--:--'}</strong></div></div>
        {error ? <div className="feedback"><span>☁</span><strong>{t.errorTitle}</strong><p>{error}</p><button onClick={() => setReload(value => value+1)}>{t.retry}</button></div> : <>
          <div className="current"><div><div className="condition">{loading ? t.loading : weather?.description || t.loading}</div><div className="temperature">{weather ? Math.round(weather.temperature) : '—'}<span>°</span></div><div className="feels">{t.feels} {weather?.feels_like ?? '—'}°C</div></div><div className="weather-art">{weather?.icon ? <img src={weather.icon} alt={weather.description}/> : <span>☀</span>}</div></div>
          <div className="divider"/><div className="details">
            <Metric icon="◉" label={t.humidity} value={weather?.humidity ?? '—'} unit="%"/><Metric icon="↗" label={t.wind} value={weather?.wind_speed?.toFixed(1) ?? '—'} unit="m/s"/><Metric icon="☼" label={t.uv} value={weather?.uv?.toFixed(1) ?? '—'} unit={weather?.uv == null ? t.oneCall : ''}/>
          </div>
          <div className="sun-row"><span>🌅 {t.sunrise} <b>{weather?.sunrise ? localTime(weather.sunrise, weather.timezone, language) : '—'}</b></span><span>🌇 {t.sunset} <b>{weather?.sunset ? localTime(weather.sunset, weather.timezone, language) : '—'}</b></span><button className={favoriteCurrent?'favorite active':'favorite'} onClick={toggleFavorite}>{favoriteCurrent?'★ '+t.favorite:'☆ '+t.saveCity}</button></div>
        </>}
      </div>
      <div className="city-row"><span className="label">{favorites.length ? t.favoriteCities : t.quickCities}</span><div>{(favorites.length ? favorites : starterCities.slice(0,4)).map(item=><span className="city-chip-wrap" key={item}><button className={city===item?'city-chip active':'city-chip'} onClick={()=>setCity(item)}>{item}</button>{favorites.length>0&&<button className="remove-city" title={t.removeFavorite(item)} aria-label={t.removeFavorite(item)} onClick={()=>setFavorites(current=>current.filter(saved=>saved.toLowerCase()!==item.toLowerCase()))}>×</button>}</span>)}<div className="add-city"><input value={newCity} onChange={e=>setNewCity(e.target.value)} onKeyDown={e=>e.key==='Enter'&&addFavorite()} placeholder={t.addCity} aria-label={t.addCity}/><button onClick={addFavorite} aria-label={t.saveFavorite}>+</button></div></div></div>
    </section>
    {!error && weather && <section className="extras">
      {allAlerts.length>0 && <div className="alerts-card"><h2>⚠️ {t.attention}</h2>{allAlerts.map((alert,index)=><article key={`${alert.event}-${index}`}><strong>{alert.event}</strong><p>{alert.description}</p><small>{alert.source}{alert.start ? ` · ${localTime(alert.start,weather.timezone,language)}` : ''}</small></article>)}</div>}
      <div className="forecast-card"><div className="section-head"><div><span className="label">{t.forecast}</span><h2>{t.comingWeather}</h2></div><div className="tabs"><button className={forecastMode==='daily'?'selected':''} onClick={()=>setForecastMode('daily')}>{t.days5}</button><button className={forecastMode==='hourly'?'selected':''} onClick={()=>setForecastMode('hourly')}>{weather.forecast_source==='hourly'?t.hourly:t.every3Hours}</button></div></div>
        {forecastMode==='daily' ? <div className="daily-list">{weather.forecast.slice(0,5).map(day=><div className="daily-item" key={day.date}><span>{new Intl.DateTimeFormat(t.dayLocale,{weekday:'short',day:'numeric',month:'short'}).format(new Date(`${day.date}T12:00:00`))}</span><img src={day.icon} alt=""/><span className="daily-desc">{day.description}</span><span className="rain">☂ {Math.round(day.rain*100)}%</span><strong>{Math.round(day.max)}° <small>{Math.round(day.min)}°</small></strong></div>)}</div> : <div className="hourly-list">{weather.hourly.slice(0,12).map(hour=><div className="hour-item" key={hour.time}><span>{localTime(hour.time,weather.timezone,language)}</span><img src={hour.icon} alt=""/><strong>{Math.round(hour.temperature)}°</strong><small>☂ {Math.round(hour.rain*100)}%</small></div>)}</div>}
        <p className="forecast-note">{weather.forecast_source==='hourly'?t.dataHourly:t.dataEvery3Hours}</p>
      </div>
      <div className="history-card"><div className="section-head"><div><span className="label">{t.historyLabel}</span><h2>{t.historyTitle}</h2></div><span className="history-count">{history.length} {t.records}</span></div>{history.length>1?<><svg className="history-chart" viewBox="0 0 600 120" role="img" aria-label={t.historyTitle}><defs><linearGradient id="chartFill" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stopColor="#7dd3fc" stopOpacity=".3"/><stop offset="100%" stopColor="#7dd3fc" stopOpacity="0"/></linearGradient></defs><polygon points={`0,120 ${chartPoints} 600,120`} fill="url(#chartFill)"/><polyline points={chartPoints} fill="none" stroke="#7dd3fc" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"/>{history.map((point,index)=><circle key={`${point.time}-${index}`} cx={history.length===1?300:index*600/(history.length-1)} cy={104-((point.temperature-minTemp)/range)*84} r="3.5" fill="#d8f3ff"/>)}</svg><div className="chart-caption"><span>{localTime(history[0].time,weather.timezone,language)}</span><strong>{minTemp.toFixed(1)}° — {maxTemp.toFixed(1)}°C</strong><span>{localTime(history[history.length-1].time,weather.timezone,language)}</span></div></>:<p className="history-empty">{t.historyEmpty}</p>}</div>
    </section>}
    <footer><span>{t.madeSimple}</span><span>{t.footer}</span></footer>
  </main>
}

function Metric({icon,label,value,unit}:{icon:string;label:string;value:string|number;unit:string}) {return <div className="detail"><span className="detail-icon">{icon}</span><div><span className="label">{label}</span><strong>{value}<small>{unit && ` ${unit}`}</small></strong></div></div>}
export default App
