import React from 'react';
import type { Config } from '../types';

interface Props {
    config: Config;
    onChange: (config: Config) => void;
    onSave: () => void;
    saving: boolean;
}

export const ConfigForm: React.FC<Props> = ({ config, onChange, onSave, saving }) => {
    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, value, type, checked } = e.target;

        let newValue: any = value;
        if (type === 'checkbox') newValue = checked;
        if (type === 'number') newValue = parseFloat(value);

        onChange({ ...config, [name]: newValue });
    };

    return (
        <form onSubmit={(e) => { e.preventDefault(); onSave(); }} className="config-form">
            <div className="form-group">
                <label htmlFor="targetUrl">Target URL</label>
                <input
                    type="url"
                    id="targetUrl"
                    name="targetUrl"
                    value={config.targetUrl}
                    onChange={handleChange}
                    required
                    placeholder="https://example.com"
                />
            </div>

            <div className="form-group">
                <label htmlFor="scaleFactor">Scale Factor</label>
                <input
                    type="number"
                    id="scaleFactor"
                    name="scaleFactor"
                    step="0.05"
                    min="0.1"
                    value={config.scaleFactor}
                    onChange={handleChange}
                    required
                />
                <small>e.g., 1.0 = 100%</small>
            </div>

            <div className="form-divider" />

            <div className="form-group checkbox-group">
                <label>
                    <input
                        type="checkbox"
                        name="autoScroll"
                        checked={config.autoScroll}
                        onChange={handleChange}
                    />
                    Enable Auto-Scroll
                </label>
            </div>

            <div className="form-group">
                <label htmlFor="scrollSpeed">Scroll Speed</label>
                <input
                    type="number"
                    id="scrollSpeed"
                    name="scrollSpeed"
                    min="10"
                    value={config.scrollSpeed}
                    onChange={handleChange}
                    required
                />
                <small>Pixels per second</small>
            </div>

            <div className="form-group">
                <label htmlFor="scrollSequence">Scroll Sequence (optional) </label>
                <input
                    type="text"
                    id="scrollSequence"
                    name="scrollSequence"
                    value={config.scrollSequence}
                    onChange={handleChange}
                    placeholder="e.g., 0-200, 500-800"
                />
            </div>

            <button type="submit" className="btn btn-primary" disabled={saving}>
                {saving ? 'Saving...' : 'Save'}
            </button>
        </form>
    );
};
