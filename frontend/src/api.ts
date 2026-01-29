import type { Config } from './types';

export const fetchConfig = async (): Promise<Config> => {
    const res = await fetch('/api/config');
    if (!res.ok) throw new Error('Failed to fetch config');
    return res.json();
};

export const saveConfig = async (config: Config): Promise<void> => {
    const res = await fetch('/api/config', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(config),
    });
    if (!res.ok) throw new Error('Failed to save config');
};
