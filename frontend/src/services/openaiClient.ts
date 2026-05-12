import OpenAI from 'openai';
import TinglyService from '@/bindings';
import { getApiBaseUrl } from '@/utils/protocol';
import { api } from '@/services/api';

const MODEL_TOKEN_KEY = 'model_token';

const resolveToken = async (): Promise<string> => {
    const stored = localStorage.getItem(MODEL_TOKEN_KEY);
    if (stored) return stored;
    if (import.meta.env.VITE_PKG_MODE === 'gui' && TinglyService) {
        try {
            const guiToken = await TinglyService.GetUserAuthToken();
            if (guiToken) return guiToken;
        } catch (err) {
            console.error('Failed to get GUI token for OpenAI client:', err);
        }
    }
    // Fall back to the backend's auto-managed model token
    try {
        const result = await api.getToken();
        if (result?.token) return result.token;
    } catch (err) {
        console.error('Failed to fetch model token:', err);
    }
    return '';
};

/**
 * Build an OpenAI SDK client targeting a tingly scenario passthrough endpoint.
 * The frontend is trusted in dev/GUI contexts, so dangerouslyAllowBrowser is
 * intentional — calls go through our own gateway, not directly to a provider.
 */
export const getOpenAIClient = async (scenario: string): Promise<OpenAI> => {
    const base = await getApiBaseUrl();
    const apiKey = await resolveToken();
    return new OpenAI({
        baseURL: `${base}/tingly/${scenario}/v1`,
        apiKey: apiKey || 'tingly',
        dangerouslyAllowBrowser: true,
    });
};
