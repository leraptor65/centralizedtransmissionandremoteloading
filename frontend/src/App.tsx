import { useEffect, useState } from 'react';
import './App.css';
import { fetchConfig, saveConfig } from './api';
import { ConfigForm } from './components/ConfigForm';
import { HistoryList } from './components/HistoryList';
import { StatusBanner } from './components/StatusBanner';
import type { Config } from './types';

const defaultConfig: Config = {
  targetUrl: 'https://github.com/leraptor65/centralizedtransmissionandremoteloading',
  scaleFactor: 1,
  autoScroll: false,
  scrollSpeed: 50,
  scrollSequence: '',
  history: []
};

function App() {
  const [config, setConfig] = useState<Config>(defaultConfig);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      const data = await fetchConfig();
      setConfig(data);
    } catch (error) {
      console.error(error);
      setMessage("Error loading configuration");
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await saveConfig(config);
      setMessage("Settings saved successfully!");
      // Reload to get updated history
      await loadConfig();
    } catch (error) {
      console.error(error);
      setMessage("Error saving settings");
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <div className="loading">Loading...</div>;

  return (
    <div className="app-container">
      <div className="card">
        <h1>
          C.T.R.L.<br />
          Settings
        </h1>
        <p className="subtitle">Configure the URL and display options below.</p>

        <StatusBanner message={message} onClear={() => setMessage(null)} />

        <ConfigForm
          config={config}
          onChange={setConfig}
          onSave={handleSave}
          saving={saving}
        />

        <div className="actions">
          <a href="/" className="btn btn-secondary">Home</a>
        </div>

        <div className="section">
          <h3>History</h3>
          <HistoryList
            history={config.history}
            onSelect={(url) => setConfig({ ...config, targetUrl: url })}
          />
        </div>

        <div className="actions">
          <button
            type="button"
            className="btn btn-danger"
            onClick={async () => {
              if (confirm('Are you sure you want to reset all settings to default?')) {
                setSaving(true);
                try {
                  const newConfig = { ...defaultConfig, history: config.history };
                  // Ensure targetUrl is exactly as requested
                  newConfig.targetUrl = 'https://github.com/leraptor65/centralizedtransmissionandremoteloading';
                  await saveConfig(newConfig);
                  setMessage("Settings reset to default!");
                  await loadConfig();
                } catch (error) {
                  console.error(error);
                  setMessage("Error resetting settings");
                } finally {
                  setSaving(false);
                }
              }
            }}
            disabled={saving}
          >
            Reset
          </button>
        </div>
      </div>
    </div>
  );
}

export default App;
