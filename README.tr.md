# ai-captcha-solver

[English](README.md)

Yapay zeka görsel API'leri kullanarak CAPTCHA çözen Go kütüphanesi. Gemini, OpenAI ve Anthropic destekler. Gemini için API anahtar havuzu desteği vardır.

## Kurulum

```bash
go get github.com/KilimcininKorOglu/ai-captcha-solver
```

Go 1.26 veya üstü gereklidir.

## Hızlı Başlangıç

### Gemini (varsayılan)

```go
solver := captcha.New(captcha.Config{
    APIKey: os.Getenv("GEMINI_API_KEY"),
})

code, err := solver.Solve(imageData)
```

### OpenAI

```go
solver := captcha.New(captcha.Config{
    Provider: "openai",
    APIKey:   os.Getenv("OPENAI_API_KEY"),
    Model:    "gpt-4o-mini",
})

code, err := solver.Solve(imageData)
```

### Anthropic

```go
solver := captcha.New(captcha.Config{
    Provider: "anthropic",
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    "claude-sonnet-4-20250514",
})

code, err := solver.Solve(imageData)
```

### Özel Base URL

Tüm sağlayıcılar proxy veya kendi barındırdığınız uç noktalar için özel base URL destekler:

```go
solver := captcha.New(captcha.Config{
    Provider: "openai",
    BaseURL:  "https://my-proxy.example.com",
    APIKey:   os.Getenv("API_KEY"),
    Model:    "gpt-4o-mini",
})
```

## API Anahtar Havuzu (yalnızca Gemini)

Rate limit'e takılmamak için istekleri birden fazla API anahtarına dağıtın. Anahtarlar sırayla kullanılır. HTTP 429 alındığında, ilgili anahtar bekleme süresine girer ve diğer anahtarlar istekleri karşılamaya devam eder. Tüm anahtarlar bekleme süresindeyse, en erken hazır olacak anahtarı bekler.

```go
solver := captcha.New(captcha.Config{
    APIKeys: []string{
        os.Getenv("GEMINI_KEY_1"),
        os.Getenv("GEMINI_KEY_2"),
        os.Getenv("GEMINI_KEY_3"),
    },
})
```

Solver birden fazla goroutine'den eş zamanlı kullanım için güvenlidir.

Anahtar havuzu yalnızca Gemini sağlayıcısı için kullanılabilir. OpenAI ve Anthropic tek anahtar kullanır.

## Yapılandırma

| Alan       | Tip      | Varsayılan              | Açıklama                                                                              |
|------------|----------|-------------------------|---------------------------------------------------------------------------------------|
| Provider   | string   | `gemini`                | Yapay zeka sağlayıcısı: `gemini`, `openai`, `anthropic`                               |
| BaseURL    | string   | Sağlayıcı varsayılanı   | Özel API base URL'i                                                                   |
| APIKey     | string   |                         | Tek API anahtarı                                                                      |
| APIKeys    | []string |                         | Anahtar havuzu (yalnızca Gemini, APIKey'e göre öncelikli)                             |
| Model      | string   | `gemini-2.5-flash-lite` | Model adı (OpenAI ve Anthropic için zorunlu, Gemini için isteğe bağlı)                |
| Prompt     | string   | Genel CAPTCHA prompt'u  | Yapay zekaya gönderilen özel prompt                                                   |
| MaxRetries | int      | 5                       | Rate limit dışı hatalar için maksimum deneme (429 sayılmaz, 5dk deadline ile sınırlı) |

## Sağlayıcı Varsayılanları

| Sağlayıcı | Varsayılan Base URL                                | Varsayılan Model        | Anahtar Havuzu |
|-----------|----------------------------------------------------|-------------------------|----------------|
| gemini    | `https://generativelanguage.googleapis.com/v1beta` | `gemini-2.5-flash-lite` | Evet           |
| openai    | `https://api.openai.com`                           | (zorunlu)               | Hayır          |
| anthropic | `https://api.anthropic.com`                        | (zorunlu)               | Hayır          |

## Rate Limit Yönetimi

1. HTTP 429 alındığında, `Retry-After` yanıt başlığındaki süre kadar bekler; başlık yoksa 60 saniye bekler
2. Gemini anahtar havuzunda: ilgili anahtar beklemeye girer, diğer anahtarlar kullanılabilir kalır
3. OpenAI/Anthropic: bekleme süresi boyunca uyur, sonra tekrar dener
4. HTTP 401/403: Gemini anahtarı 24 saat devre dışı bırakır; OpenAI/Anthropic kalıcı hata döndürür
5. Toplam `Solve` süresi 5 dakikayla sınırlıdır

## Doğrulama

Solver oluşturmadan önce yapılandırmayı kontrol etmek için `captcha.Validate(cfg)` kullanın. API anahtarı eksikse, Gemini dışı bir sağlayıcıyla anahtar havuzu kullanılıyorsa veya OpenAI/Anthropic için model belirtilmemişse hata döndürür.

## Nasıl Çalışır

1. CAPTCHA görselini base64 olarak kodlar
2. Yapılandırılmış yapay zeka API'sine metin prompt'u ile birlikte gönderir
3. Yanıtı parse eder, küçük harfe çevirir, alfanümerik olmayan karakterleri temizler, uzunluğu doğrular (4-8 karakter)
4. Temizlenmiş küçük harfli CAPTCHA metnini döndürür

## Ücretsiz API Sağlayıcıları

### Gemini (Google)

Rate limitli ücretsiz katman. [Google AI Studio](https://aistudio.google.com/app/apikey) üzerinden ücretsiz API anahtarı alabilirsiniz.

### NVIDIA NIM (HermesTech)

[HermesTech](https://nvidia.srv.hermestech.uk) üzerinden Anthropic uyumlu API ile birden fazla görsel modele ücretsiz erişim. Kredi kartı gerekmez.

```go
solver := captcha.New(captcha.Config{
    Provider: "anthropic",
    BaseURL:  "https://nvidia.srv.hermestech.uk",
    APIKey:   "hermes-api-anahtariniz",
    Model:    "microsoft/phi-4-multimodal-instruct",
})
```

CAPTCHA çözümü ile test edilmiş görsel modeller:

| Model                                     | Hız    | Başarı |
|-------------------------------------------|--------|--------|
| `microsoft/phi-4-multimodal-instruct`     | ~400ms | En iyi |
| `nvidia/nemotron-nano-12b-v2-vl`          | ~600ms | En iyi |
| `qwen/qwen3.5-122b-a10b`                  | ~700ms | En iyi |
| `meta/llama-4-maverick-17b-128e-instruct` | ~2s    | İyi    |
| `meta/llama-3.2-11b-vision-instruct`      | ~4s    | İyi    |
| `google/gemma-3n-e2b-it`                  | ~6s    | İyi    |
| `google/gemma-3n-e4b-it`                  | ~9s    | İyi    |
| `meta/llama-3.2-90b-vision-instruct`      | ~9s    | İyi    |

### Ücretli API Anahtarları

- OpenAI: [OpenAI Platform](https://platform.openai.com/api-keys)
- Anthropic: [Anthropic Console](https://console.anthropic.com/settings/keys)

## Lisans

MIT
