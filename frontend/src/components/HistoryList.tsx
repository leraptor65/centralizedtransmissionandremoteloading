import React from 'react';
import type { HistoryItem } from '../types';

interface Props {
    history: HistoryItem[];
    onSelect: (url: string) => void;
}

export const HistoryList: React.FC<Props> = ({ history, onSelect }) => {
    if (history.length === 0) {
        return <div className="history-empty">No history yet</div>;
    }

    return (
        <div className="history-list">
            {history.map((item, index) => (
                <div
                    key={`${index}-${item.timestamp}`}
                    className="history-item"
                    onClick={() => onSelect(item.url)}
                >
                    <span className="history-url" title={item.url}>
                        {item.url.length > 50 ? item.url.substring(0, 47) + '...' : item.url}
                    </span>
                    <span className="history-time">
                        {new Date(item.timestamp).toLocaleDateString()}
                    </span>
                </div>
            ))}
        </div>
    );
};
