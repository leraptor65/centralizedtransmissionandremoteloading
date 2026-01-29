export interface HistoryItem {
    url: string;
    timestamp: number;
}

export interface Config {
    targetUrl: string;
    scaleFactor: number;
    autoScroll: boolean;
    scrollSpeed: number;
    scrollSequence: string;
    history: HistoryItem[];
}
