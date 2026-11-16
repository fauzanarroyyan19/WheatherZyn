import React, { useState } from "react";
import axios from "axios";
import WeatherCard from "../components/WeatherCard";
import CitySelector from "../components/CitySelector";

const Dashboard = () => {
  const [weather, setWeather] = useState(null);

  const getWeather = async (city) => {
    try {
      const res = await axios.get(`http://localhost:8080/api/weather?city=${city}`);
      setWeather(res.data);
    } catch (err) {
      console.error("Gagal ambil data cuaca:", err);
    }
  };

  return (
    <div className="bg-white/10 p-10 rounded-2xl shadow-2xl w-[400px] backdrop-blur-lg">
      <h1 className="text-3xl font-bold mb-5 text-center">🌤️ WeatherNow Dashboard</h1>
      <CitySelector onSelectCity={getWeather} />
      {weather && <WeatherCard data={weather} />}
      <p className="text-center mt-5 text-sm opacity-80">
        by <a href="https://instagram.com/fznarroyyan" className="underline">Fauzan Arroyyan</a>
      </p>
    </div>
  );
};

export default Dashboard;
