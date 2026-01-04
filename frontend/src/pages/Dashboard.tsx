import React from 'react';

const Dashboard: React.FC = () => {
  return (
    <div className="dashboard">
      <h2>Dashboard</h2>
      <div className="stats-grid">
        <div className="stat-card">
          <h3>Total Games</h3>
          <p>0</p>
        </div>
        <div className="stat-card">
          <h3>Downloading</h3>
          <p>0</p>
        </div>
        <div className="stat-card">
          <h3>Platforms</h3>
          <p>0</p>
        </div>
      </div>
    </div>
  );
};

export default Dashboard;
