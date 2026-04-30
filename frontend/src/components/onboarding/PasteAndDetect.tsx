import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    CircularProgress,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import AutoFixHighIcon from '@mui/icons-material/AutoFixHigh';
import ProviderIcon from '@/components/ProviderIcon';
import {extractOnboardingCandidates, type OnboardingCandidate} from '@/services/onboardingExtract';
import type {EnhancedProviderFormData} from '@/components/ProviderFormDialog';

interface PasteAndDetectProps {
    onPick: (prefill: EnhancedProviderFormData) => void;
    onManualFill: () => void;
}

const PLACEHOLDER = `# .env
OPENAI_API_KEY=sk-proj-...
OPENAI_BASE_URL=https://api.openai.com/v1

# or paste a curl snippet
curl https://api.anthropic.com/v1/messages \\
  -H "x-api-key: sk-ant-..."
`;

const maskToken = (raw?: string) => {
    if (!raw) return '';
    if (raw.length <= 8) return '•'.repeat(raw.length);
    return raw.slice(0, 4) + '…' + raw.slice(-4);
};

const confidenceLabel = (c: number) => {
    if (c >= 0.8) return 'high';
    if (c >= 0.5) return 'medium';
    return 'low';
};

const confidenceColor = (c: number): 'success' | 'warning' | 'default' => {
    if (c >= 0.8) return 'success';
    if (c >= 0.5) return 'warning';
    return 'default';
};

const PasteAndDetect: React.FC<PasteAndDetectProps> = ({onPick, onManualFill}) => {
    const {t} = useTranslation();
    const [input, setInput] = useState('');
    const [loading, setLoading] = useState(false);
    const [candidates, setCandidates] = useState<OnboardingCandidate[] | null>(null);
    const [warnings, setWarnings] = useState<string[]>([]);
    const [error, setError] = useState<string | null>(null);

    const handleDetect = async () => {
        setLoading(true);
        setError(null);
        try {
            const res = await extractOnboardingCandidates(input);
            if (!res.success) {
                setError(res.error || 'Extraction failed');
                setCandidates([]);
                setWarnings([]);
                return;
            }
            setCandidates(res.candidates);
            setWarnings(res.warnings);
        } finally {
            setLoading(false);
        }
    };

    const handleUse = (c: OnboardingCandidate) => {
        const apiStyle = (c.api_style === 'anthropic' ? 'anthropic' : 'openai') as 'openai' | 'anthropic';
        const protocols: ('openai' | 'anthropic')[] = (c.protocols || []).filter(
            (p): p is 'openai' | 'anthropic' => p === 'openai' || p === 'anthropic',
        );
        onPick({
            name: c.name,
            apiBase: c.base_url || '',
            apiStyle,
            token: c.token || '',
            enabled: true,
            protocols: protocols.length ? protocols : [apiStyle],
        });
    };

    return (
        <Box>
            <TextField
                fullWidth
                multiline
                minRows={6}
                maxRows={14}
                value={input}
                onChange={e => setInput(e.target.value)}
                placeholder={PLACEHOLDER}
                spellCheck={false}
                inputProps={{style: {fontFamily: 'monospace', fontSize: 13}}}
            />

            <Stack direction="row" spacing={1.5} sx={{mt: 1.5}}>
                <Button
                    variant="contained"
                    startIcon={loading ? <CircularProgress size={16} color="inherit"/> : <AutoFixHighIcon/>}
                    onClick={handleDetect}
                    disabled={loading || !input.trim()}
                >
                    {t('onboarding.paste.detectButton', {defaultValue: 'Detect'})}
                </Button>
                <Button variant="text" onClick={onManualFill}>
                    {t('onboarding.paste.manualFill', {defaultValue: 'Fill in manually'})}
                </Button>
            </Stack>

            {error && (
                <Alert severity="error" sx={{mt: 2}}>
                    {error}
                </Alert>
            )}

            {warnings.map((w, i) => (
                <Alert key={i} severity="warning" sx={{mt: 2}}>
                    {w}
                </Alert>
            ))}

            {candidates !== null && (
                <Box sx={{mt: 2}}>
                    {candidates.length === 0 ? (
                        <Alert severity="info">
                            {t('onboarding.paste.noMatch', {
                                defaultValue: 'Could not detect a known provider. You can fill in the form manually.',
                            })}
                        </Alert>
                    ) : (
                        <Stack spacing={1.5}>
                            {candidates.map(c => (
                                <Card key={c.provider_id} variant="outlined">
                                    <CardContent sx={{py: 1.5, '&:last-child': {pb: 1.5}}}>
                                        <Stack direction="row" spacing={2} alignItems="center">
                                            <ProviderIcon identifier={c.icon || c.provider_id} size={32}/>
                                            <Box sx={{flex: 1, minWidth: 0}}>
                                                <Stack direction="row" spacing={1} alignItems="center" sx={{mb: 0.5}}>
                                                    <Typography variant="subtitle2" fontWeight={600}>
                                                        {c.name}
                                                    </Typography>
                                                    <Chip
                                                        size="small"
                                                        label={`${confidenceLabel(c.confidence)} · ${Math.round(c.confidence * 100)}%`}
                                                        color={confidenceColor(c.confidence)}
                                                        sx={{height: 18, fontSize: '0.65rem'}}
                                                    />
                                                    {c.api_style && (
                                                        <Chip
                                                            size="small"
                                                            label={c.api_style}
                                                            variant="outlined"
                                                            sx={{height: 18, fontSize: '0.65rem'}}
                                                        />
                                                    )}
                                                </Stack>
                                                <Typography
                                                    variant="caption"
                                                    color="text.secondary"
                                                    sx={{display: 'block', wordBreak: 'break-all'}}
                                                >
                                                    {c.base_url || '—'}
                                                    {c.token ? ` · key: ${maskToken(c.token)}` : ''}
                                                </Typography>
                                                {c.match_reasons && c.match_reasons.length > 0 && (
                                                    <Stack direction="row" spacing={0.5} sx={{mt: 0.5, flexWrap: 'wrap', gap: 0.5}}>
                                                        {c.match_reasons.map((r, i) => (
                                                            <Chip
                                                                key={i}
                                                                label={r}
                                                                size="small"
                                                                sx={{height: 16, fontSize: '0.6rem'}}
                                                            />
                                                        ))}
                                                    </Stack>
                                                )}
                                            </Box>
                                            <Button variant="contained" size="small" onClick={() => handleUse(c)}>
                                                {t('onboarding.candidate.useThis', {defaultValue: 'Use this'})}
                                            </Button>
                                        </Stack>
                                    </CardContent>
                                </Card>
                            ))}
                        </Stack>
                    )}
                </Box>
            )}
        </Box>
    );
};

export default PasteAndDetect;
