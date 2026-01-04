using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Threading.Tasks;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Playerr.Core.MetadataSource.Igdb
{
    /// <summary>
    /// Cliente para IGDB API - Similar a MovieDB en Radarr
    /// Requiere Client ID y Client Secret de Twitch
    /// </summary>
    public class IgdbClient
    {
        private readonly HttpClient _httpClient;
        private string _clientId;
        private string _clientSecret;
        private string? _accessToken;
        private DateTime _tokenExpiration;

        private const string IgdbApiUrl = "https://api.igdb.com/v4";
        private const string TwitchAuthUrl = "https://id.twitch.tv/oauth2/token";

        public IgdbClient(string clientId, string clientSecret)
        {
            _httpClient = new HttpClient();
            _clientId = clientId;
            _clientSecret = clientSecret;
        }

        public bool HasValidCredentials =>
            !string.IsNullOrWhiteSpace(_clientId) &&
            !string.IsNullOrWhiteSpace(_clientSecret);

        public void UpdateCredentials(string clientId, string clientSecret)
        {
            _clientId = clientId;
            _clientSecret = clientSecret;
            _accessToken = null;
            _tokenExpiration = DateTime.MinValue;
        }

        private async Task EnsureAuthenticatedAsync()
        {
            if (!HasValidCredentials)
            {
                throw new InvalidOperationException("IGDB credentials are not configured");
            }

            if (!string.IsNullOrEmpty(_accessToken) && DateTime.UtcNow < _tokenExpiration)
                return;

            var request = new HttpRequestMessage(HttpMethod.Post, 
                $"{TwitchAuthUrl}?client_id={_clientId}&client_secret={_clientSecret}&grant_type=client_credentials");

            var response = await _httpClient.SendAsync(request);
            if (!response.IsSuccessStatusCode)
            {
                var errorContent = await response.Content.ReadAsStringAsync();
                Console.WriteLine($"[IgdbClient] Auth Error ({response.StatusCode}): {errorContent}");
                throw new HttpRequestException($"IGDB Auth Failed: {response.StatusCode} - {errorContent}");
            }

            var content = await response.Content.ReadAsStringAsync();
            var authResponse = JsonSerializer.Deserialize<TwitchAuthResponse>(content);

            _accessToken = authResponse?.AccessToken;
            _tokenExpiration = DateTime.UtcNow.AddSeconds(authResponse?.ExpiresIn ?? 3600);
        }

        public async Task<List<IgdbGame>> SearchGamesAsync(string query, int? platformId = null, string? lang = null, string? externalId = null)
        {
            await EnsureAuthenticatedAsync();

            string requestBody;
            
            if (!string.IsNullOrEmpty(externalId))
            {
                // Search by external identifier (e.g., CUSA code for PS4)
                requestBody = $@"
                    fields name, summary, storyline, cover.url, cover.image_id, 
                           screenshots.url, screenshots.image_id, artworks.url, artworks.image_id,
                           first_release_date, genres.name, involved_companies.company.name,
                           rating, rating_count, platforms.name,
                           external_games.category, external_games.uid;
                    where external_games.uid = ""{externalId}"";
                    limit 10;
                ";
                
                var resultsByExternalId = await ExecuteQueryAsync<IgdbGame>("games", requestBody);
                if (resultsByExternalId.Any()) return resultsByExternalId;
            }

            // Fallback to name-based search with platform filter
            requestBody = $@"
                search ""{query}"";
                fields name, summary, storyline, cover.url, cover.image_id, 
                       screenshots.url, screenshots.image_id, artworks.url, artworks.image_id,
                       first_release_date, genres.name, involved_companies.company.name,
                       rating, rating_count, platforms.name,
                       external_games.category, external_games.uid;
                {(platformId.HasValue ? $"where platforms = ({platformId});" : "")}
                limit 20;
            ";

            return await ExecuteQueryAsync<IgdbGame>("games", requestBody);
        }

        public async Task<IgdbGame?> GetGameByIdAsync(int igdbId, string? lang = null)
        {
            var results = await GetGamesByIdsAsync(new[] { igdbId }, lang);
            return results.FirstOrDefault();
        }

        public async Task<List<IgdbGame>> GetGamesByIdsAsync(IEnumerable<int> igdbIds, string? lang = null)
        {
            if (igdbIds == null || !igdbIds.Any()) return new List<IgdbGame>();

            await EnsureAuthenticatedAsync();

            var idsFilter = string.Join(",", igdbIds);
            var requestBody = $@"
                fields name, summary, storyline, cover.url, cover.image_id,
                       screenshots.url, screenshots.image_id, artworks.url, artworks.image_id,
                       first_release_date, genres.name, involved_companies.company.name,
                       involved_companies.developer, involved_companies.publisher,
                       rating, rating_count, platforms.name,
                       external_games.category, external_games.uid;
                where id = ({idsFilter});
                limit {igdbIds.Count()};
            ";

            return await ExecuteQueryAsync<IgdbGame>("games", requestBody);
        }

        public async Task<List<IgdbPlatform>> GetPlatformsAsync()
        {
            await EnsureAuthenticatedAsync();

            var requestBody = @"
                fields name, slug, platform_logo.url;
                where category = (1,2,3,4,5,6);
                limit 100;
                sort name asc;
            ";

            return await ExecuteQueryAsync<IgdbPlatform>("platforms", requestBody);
        }

        private async Task<List<T>> ExecuteQueryAsync<T>(string endpoint, string body)
        {
            var request = new HttpRequestMessage(HttpMethod.Post, $"{IgdbApiUrl}/{endpoint}")
            {
                Content = new StringContent(body, Encoding.UTF8, "text/plain")
            };

            request.Headers.Add("Client-ID", _clientId);
            request.Headers.Add("Authorization", $"Bearer {_accessToken}");

            var response = await _httpClient.SendAsync(request);
            
            if (!response.IsSuccessStatusCode)
            {
                var errorContent = await response.Content.ReadAsStringAsync();
                Console.WriteLine($"[IgdbClient] Query Error ({response.StatusCode}) for endpoint {endpoint}: {errorContent}");
                Console.WriteLine($"[IgdbClient] Request Body: {body}");
                throw new HttpRequestException($"IGDB Query Failed: {response.StatusCode} - {errorContent}");
            }

            var content = await response.Content.ReadAsStringAsync();
            return JsonSerializer.Deserialize<List<T>>(content, new JsonSerializerOptions 
            { 
                PropertyNameCaseInsensitive = true 
            }) ?? new List<T>();
        }

        public static string GetImageUrl(string imageId, ImageSize size = ImageSize.CoverBig)
        {
            var sizeStr = size switch
            {
                ImageSize.CoverSmall => "t_cover_small",
                ImageSize.CoverBig => "t_cover_big",
                ImageSize.Screenshot => "t_screenshot_med",
                ImageSize.ScreenshotHuge => "t_screenshot_huge",
                ImageSize.Thumb => "t_thumb",
                ImageSize.Micro => "t_micro",
                ImageSize.HD => "t_1080p",
                _ => "t_cover_big"
            };

            return $"https://images.igdb.com/igdb/image/upload/{sizeStr}/{imageId}.jpg";
        }
    }

    public class TwitchAuthResponse
    {
        [JsonPropertyName("access_token")]
        public string AccessToken { get; set; } = string.Empty;

        [JsonPropertyName("expires_in")]
        public int ExpiresIn { get; set; }

        [JsonPropertyName("token_type")]
        public string TokenType { get; set; } = string.Empty;
    }

    public class IgdbGame
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public string? Summary { get; set; }
        public string? Storyline { get; set; }
        public IgdbCover? Cover { get; set; }
        public List<IgdbImage> Screenshots { get; set; } = new();
        public List<IgdbImage> Artworks { get; set; } = new();
        
        [JsonPropertyName("first_release_date")]
        public long? FirstReleaseDate { get; set; }
        
        public List<IgdbGenre> Genres { get; set; } = new();
        
        [JsonPropertyName("involved_companies")]
        public List<IgdbInvolvedCompany> InvolvedCompanies { get; set; } = new();
        
        public double? Rating { get; set; }
        
        [JsonPropertyName("rating_count")]
        public int? RatingCount { get; set; }
        
        public List<IgdbPlatformRef> Platforms { get; set; } = new();

        [JsonPropertyName("game_localizations")]
        public List<IgdbLocalization> Localizations { get; set; } = new();

        [JsonPropertyName("external_games")]
        public List<IgdbExternalGame> ExternalGames { get; set; } = new();
    }

    public class IgdbExternalGame
    {
        public int Category { get; set; }
        public string? Uid { get; set; }
    }

    public class IgdbLocalization
    {
        public int Id { get; set; }
        public string? Name { get; set; }
        public string? Summary { get; set; }
        public string? Storyline { get; set; }
        public int Language { get; set; }
    }

    public class IgdbCover
    {
        public int Id { get; set; }
        
        [JsonPropertyName("image_id")]
        public string ImageId { get; set; } = string.Empty;
        
        public string? Url { get; set; }
    }

    public class IgdbImage
    {
        public int Id { get; set; }
        
        [JsonPropertyName("image_id")]
        public string ImageId { get; set; } = string.Empty;
        
        public string? Url { get; set; }
    }

    public class IgdbGenre
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
    }

    public class IgdbInvolvedCompany
    {
        public IgdbCompany Company { get; set; } = new();
        public bool Developer { get; set; }
        public bool Publisher { get; set; }
    }

    public class IgdbCompany
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
    }

    public class IgdbPlatform
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public string Slug { get; set; } = string.Empty;
        
        [JsonPropertyName("platform_logo")]
        public IgdbImage? PlatformLogo { get; set; }
    }

    public class IgdbPlatformRef
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
    }

    public enum ImageSize
    {
        CoverSmall,
        CoverBig,
        Screenshot,
        ScreenshotHuge,
        Thumb,
        Micro,
        HD
    }
}
