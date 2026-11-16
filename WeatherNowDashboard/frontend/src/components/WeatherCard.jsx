import React from "react";

const WeatherCard = ({ data }) => {
  return (
    <div className="text-center bg-white/20 p-5 rounded-xl mt-4">
      <h2 className="text-2xl font-semibold mb-2">🌆 {data.city}</h2>
      <p className="text-lg">Suhu: {data.temperature}°C</p>
      <p className="text-md capitalize">Kondisi: {data.status}</p>
    </div>
  );
};

export default WeatherCard;
