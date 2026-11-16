import React from "react";

const cities = ["Jakarta", "Bandung", "Bekasi", "Surabaya", "Bali", "Makassar"];

const CitySelector = ({ onSelectCity }) => {
  return (
    <div className="flex flex-col items-center mb-6">
      <select
        onChange={(e) => onSelectCity(e.target.value)}
        className="text-black p-2 rounded-md outline-none"
      >
        <option value="">Pilih Kota</option>
        {cities.map((city) => (
          <option key={city} value={city.toLowerCase()}>
            {city}
          </option>
        ))}
      </select>
    </div>
  );
};

export default CitySelector;
