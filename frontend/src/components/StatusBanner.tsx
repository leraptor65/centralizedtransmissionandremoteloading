import React, { useEffect, useState } from 'react';

interface Props {
    message: string | null;
    onClear: () => void;
}

export const StatusBanner: React.FC<Props> = ({ message, onClear }) => {
    const [visible, setVisible] = useState(false);

    useEffect(() => {
        if (message) {
            setVisible(true);
            const timer = setTimeout(() => {
                setVisible(false);
                setTimeout(onClear, 500); // Wait for fade out
            }, 3000);
            return () => clearTimeout(timer);
        }
    }, [message, onClear]);

    if (!message && !visible) return null;

    return (
        <div className={`status-banner ${visible ? 'visible' : ''}`}>
            {message}
        </div>
    );
};
