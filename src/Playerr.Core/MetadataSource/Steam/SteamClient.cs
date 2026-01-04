using System;
using System.Net.Http;
using System.Text.Json;
using System.Threading.Tasks;
using System.Collections.Generic;
using System.Linq;
using Playerr.Core.Configuration;
using Playerr.Core.Games;

namespace Playerr.Core.MetadataSource.Steam
{
    public class SteamClient
    {
        private readonly HttpClient _httpClient;
        private readonly ConfigurationService _configService;

        public SteamClient(ConfigurationService configService)
        {
            _httpClient = new HttpClient();
            _httpClient.DefaultRequestHeaders.Add("User-Agent", "Playerr/1.0");
            _configService = configService;
        }

        private string GetApiKey()
        {
            var settings = _configService.LoadSteamSettings();
            if (string.IsNullOrEmpty(settings.ApiKey))
            {
                throw new Exception("Steam API Key not configured.");
            }
            return settings.ApiKey;
        }

        public async Task<SteamPlayerProfile?> GetPlayerSummariesAsync(string steamId)
        {
            try
            {
                var apiKey = GetApiKey();
                var finalSteamId = steamId;

                // If not numeric, try to resolve vanity URL
                if (!long.TryParse(steamId, out _))
                {
                    finalSteamId = await ResolveVanityUrlAsync(steamId, apiKey);
                }

                // GetPlayerSummaries v2
                var url = $"https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002/?key={apiKey}&steamids={finalSteamId}";
                
                var response = await _httpClient.GetAsync(url);
                EnsureSuccessOrThrow(response);

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);
                
                // Navigate: response -> players -> [0]
                var players = root.RootElement.GetProperty("response").GetProperty("players");
                if (players.GetArrayLength() > 0)
                {
                    var player = players[0];
                    return new SteamPlayerProfile
                    {
                        SteamId = player.GetProperty("steamid").GetString() ?? "",
                        PersonaName = player.GetProperty("personaname").GetString() ?? "Unknown",
                        AvatarUrl = player.GetProperty("avatarfull").GetString() ?? "",
                        PersonaState = player.TryGetProperty("personastate", out var state) ? state.GetInt32() : 0,
                        GameExtraInfo = player.TryGetProperty("gameextrainfo", out var game) ? game.GetString() : null
                    };
                }
                
                return null;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error fetching player summaries: {ex.Message}");
                // Return null or throw depending on preference. 
                // For UI partial loading, returning null allows gracefully showing "Not connected" or error.
                return null; 
            }
        }

        private async Task<string> ResolveVanityUrlAsync(string vanityUrl, string apiKey)
        {
            var url = $"https://api.steampowered.com/ISteamUser/ResolveVanityURL/v0001/?key={apiKey}&vanityurl={vanityUrl}";
            var response = await _httpClient.GetAsync(url);
            EnsureSuccessOrThrow(response);

            var content = await response.Content.ReadAsStringAsync();
            var root = JsonDocument.Parse(content);
            var responseElem = root.RootElement.GetProperty("response");

            if (responseElem.GetProperty("success").GetInt32() == 1)
            {
                return responseElem.GetProperty("steamid").GetString()!;
            }

            throw new Exception($"Could not resolve Steam Vanity URL: {vanityUrl}");
        }

        public async Task<SteamLibraryStats> GetLibraryStatsAsync(string steamId)
        {
            var stats = new SteamLibraryStats();
            try
            {
                var apiKey = GetApiKey();
                var finalSteamId = steamId;

                if (!long.TryParse(steamId, out _))
                {
                    finalSteamId = await ResolveVanityUrlAsync(steamId, apiKey);
                }

                // GetOwnedGames v1
                var url = $"https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key={apiKey}&steamid={finalSteamId}&format=json&include_played_free_games=1";
                
                var response = await _httpClient.GetAsync(url);
                EnsureSuccessOrThrow(response);

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);
                
                if (root.RootElement.GetProperty("response").TryGetProperty("games", out var gamesElement))
                {
                    stats.TotalGames = root.RootElement.GetProperty("response").TryGetProperty("game_count", out var count) ? count.GetInt32() : 0;
                    
                    foreach (var gameElem in gamesElement.EnumerateArray())
                    {
                        if (gameElem.TryGetProperty("playtime_forever", out var playtime))
                        {
                            stats.TotalMinutesPlayed += playtime.GetInt32();
                        }
                    }
                }
                
                return stats;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error calculating library stats: {ex.Message}");
                return new SteamLibraryStats(); // Return empty stats on failure
            }
        }

        public async Task<List<SteamRecentGame>> GetRecentlyPlayedGamesAsync(string steamId)
        {
            var recentGames = new List<SteamRecentGame>();
            try
            {
                var apiKey = GetApiKey();
                // GetRecentlyPlayedGames v1
                var url = $"https://api.steampowered.com/IPlayerService/GetRecentlyPlayedGames/v0001/?key={apiKey}&steamid={steamId}&format=json&count=5";
                
                var response = await _httpClient.GetAsync(url);
                if (!response.IsSuccessStatusCode) return recentGames;

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);
                
                if (root.RootElement.GetProperty("response").TryGetProperty("games", out var gamesElement))
                {
                    var tasks = new List<Task<SteamRecentGame>>();

                    foreach (var gameElem in gamesElement.EnumerateArray())
                    {
                        var appId = gameElem.GetProperty("appid").GetInt32();
                        var name = gameElem.GetProperty("name").GetString() ?? $"App {appId}";
                        var playtime2Weeks = gameElem.TryGetProperty("playtime_2weeks", out var p2w) ? p2w.GetInt32() : 0;
                        var playtimeForever = gameElem.GetProperty("playtime_forever").GetInt32();

                        tasks.Add(ProcessGameAsync(steamId, appId, name, playtime2Weeks, playtimeForever, apiKey));
                    }
                    
                    recentGames = (await Task.WhenAll(tasks)).ToList();
                }
                
                return recentGames;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error fetching recent games: {ex.Message}");
                return recentGames;
            }
        }

        private async Task<SteamRecentGame> ProcessGameAsync(string steamId, int appId, string name, int playtime2Weeks, int playtimeForever, string apiKey)
        {
            var game = new SteamRecentGame
            {
                AppId = appId,
                Name = name,
                Playtime2Weeks = playtime2Weeks,
                PlaytimeForever = playtimeForever,
                // Use High-Res Header Image (460x215) instead of the tiny icon
                IconUrl = $"https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/{appId}/header.jpg"
            };

            // Run achievements and news fetch in parallel
            var achievementsTask = GetPlayerAchievementsAsync(steamId, appId, apiKey);
            var newsTask = GetNewsForAppAsync(appId);

            await Task.WhenAll(achievementsTask, newsTask);

            var (achieved, total) = await achievementsTask;
            game.Achieved = achieved;
            game.TotalAchievements = total;
            game.LatestNews = await newsTask;

            return game;
        }

        private async Task<SteamNewsItem?> GetNewsForAppAsync(int appId)
        {
            try
            {
                // GetNewsForApp v2
                var url = $"https://api.steampowered.com/ISteamNews/GetNewsForApp/v0002/?appid={appId}&count=1&maxlength=300&format=json";
                var response = await _httpClient.GetAsync(url);
                
                if (!response.IsSuccessStatusCode) return null;

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);
                
                if (!root.RootElement.TryGetProperty("appnews", out var appnews)) return null;
                if (!appnews.TryGetProperty("newsitems", out var newsitems) || newsitems.GetArrayLength() == 0) return null;

                var news = newsitems[0];
                return new SteamNewsItem
                {
                    Title = news.GetProperty("title").GetString() ?? "Update",
                    Url = news.GetProperty("url").GetString() ?? "",
                    FeedLabel = news.GetProperty("feedlabel").GetString() ?? "News",
                    // Date is unix timestamp
                    Date = DateTimeOffset.FromUnixTimeSeconds(news.GetProperty("date").GetInt64()).DateTime
                };
            }
            catch
            {
                return null;
            }
        }

        private async Task<(int achieved, int total)> GetPlayerAchievementsAsync(string steamId, int appId, string apiKey)
        {
            try
            {
                var url = $"https://api.steampowered.com/ISteamUserStats/GetPlayerAchievements/v0001/?appid={appId}&key={apiKey}&steamid={steamId}";
                var response = await _httpClient.GetAsync(url);
                
                if (!response.IsSuccessStatusCode) return (0, 0);

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);
                
                if (!root.RootElement.TryGetProperty("playerstats", out var stats)) return (0, 0);
                if (!stats.TryGetProperty("achievements", out var achievements)) return (0, 0);

                int achievedCount = 0;
                int totalCount = achievements.GetArrayLength();

                foreach (var ach in achievements.EnumerateArray())
                {
                    if (ach.TryGetProperty("achieved", out var achieved) && achieved.GetInt32() == 1)
                    {
                        achievedCount++;
                    }
                }

                return (achievedCount, totalCount);
            }
            catch
            {
                return (0, 0);
            }
        }

        public async Task<List<Game>> GetOwnedGamesAsync(string steamId)
        {
            var games = new List<Game>();
            try
            {
                var apiKey = GetApiKey();
                var finalSteamId = steamId;

                // If not numeric, try to resolve vanity URL
                if (!long.TryParse(steamId, out _))
                {
                    finalSteamId = await ResolveVanityUrlAsync(steamId, apiKey);
                }

                // GetOwnedGames v1 - include_appinfo=1 to get name and img_icon_url
                var url = $"https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key={apiKey}&steamid={finalSteamId}&format=json&include_appinfo=1&include_played_free_games=1";
                
                var response = await _httpClient.GetAsync(url);
                EnsureSuccessOrThrow(response);

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);
                
                if (root.RootElement.GetProperty("response").TryGetProperty("games", out var gamesElement))
                {
                    foreach (var gameElem in gamesElement.EnumerateArray())
                    {
                        var appid = gameElem.GetProperty("appid").GetInt32();
                        var name = gameElem.GetProperty("name").GetString();
                        var imgIconUrl = gameElem.TryGetProperty("img_icon_url", out var icon) ? icon.GetString() : null;
                        
                        // Construct basic Game object
                        var game = new Game
                        {
                            Title = name ?? $"Steam App {appid}",
                            SteamId = appid,
                            PlatformId = 1, // Accessing Platform ID 1 (PC) assumption or logic needed? Let's assume 1 is default/PC.
                            Status = GameStatus.Announced, // Or Owned? Status enum has TBA, Announced, Released, Downloading, Downloaded, Missing. Maybe "Released" is best fit for owned games?
                            Added = DateTime.UtcNow,
                            Monitored = true
                        };
                        
                        // Icon URL format: http://media.steampowered.com/steamcommunity/public/images/apps/{appid}/{hash}.jpg
                        if (!string.IsNullOrEmpty(imgIconUrl))
                        {
                             // We might want to fetch higher res images from IGDB later, but this is a start.
                        }

                        games.Add(game);
                    }
                }
                
                return games;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error fetching owned games: {ex.Message}");
                throw new Exception($"Failed to sync Steam library: {ex.Message}");
            }
        }

        public async Task<SteamGameDetails?> GetGameDetailsAsync(string steamId, string lang = "en")
        {
            try
            {
                // Map ISO codes to Steam language parameters
                // Steam uses full language names or specific codes
                var steamLang = MapToSteamLanguage(lang);
                var url = $"https://store.steampowered.com/api/appdetails?appids={steamId}&l={steamLang}";
                
                var response = await _httpClient.GetAsync(url);
                if (!response.IsSuccessStatusCode) return null;

                var content = await response.Content.ReadAsStringAsync();
                var root = JsonDocument.Parse(content);

                if (!root.RootElement.TryGetProperty(steamId, out var gameProp)) return null;
                if (!gameProp.TryGetProperty("success", out var success) || !success.GetBoolean()) return null;
                if (!gameProp.TryGetProperty("data", out var data)) return null;

                return JsonSerializer.Deserialize<SteamGameDetails>(data.GetRawText(), new JsonSerializerOptions
                {
                    PropertyNameCaseInsensitive = true
                });
            }
            catch
            {
                return null;
            }
        }

        private string MapToSteamLanguage(string lang)
        {
            return lang.ToLower() switch
            {
                "es" => "spanish",
                "fr" => "french",
                "de" => "german",
                "ru" => "russian",
                "zh" => "schinese", // Simplified Chinese
                "ja" => "japanese",
                _ => "english"
            };
        }

        private void EnsureSuccessOrThrow(HttpResponseMessage response)
        {
            if (response.StatusCode == System.Net.HttpStatusCode.Unauthorized) // 401
            {
                throw new Exception("Unauthorized (401). Invalid Steam API Key. Please verify your API Key has no extra spaces and is correct.");
            }
            if (response.StatusCode == System.Net.HttpStatusCode.Forbidden) // 403
            {
                throw new Exception("Access Denied (403). Possible reasons: \n1. Invalid API Key \n2. Steam Profile/Game Details set to Private (must be Public).");
            }
            response.EnsureSuccessStatusCode();
        }
    }

    public class SteamGameDetails
    {
        public string Name { get; set; } = string.Empty;
        public string DetailedDescription { get; set; } = string.Empty;
        public string ShortDescription { get; set; } = string.Empty;
        public string AboutTheGame { get; set; } = string.Empty;
    }

    public class SteamPlayerProfile
    {
        public string SteamId { get; set; } = string.Empty;
        public string PersonaName { get; set; } = string.Empty;
        public string AvatarUrl { get; set; } = string.Empty;
        public int PersonaState { get; set; }
        public string? GameExtraInfo { get; set; }
    }

    public class SteamLibraryStats
    {
        public int TotalGames { get; set; }
        public int TotalMinutesPlayed { get; set; }
        public double TotalHoursPlayed => Math.Round(TotalMinutesPlayed / 60.0, 1);
    }

    public class SteamRecentGame
    {
        public int AppId { get; set; }
        public string Name { get; set; } = string.Empty;
        public int Playtime2Weeks { get; set; }
        public int PlaytimeForever { get; set; }
        public string? IconUrl { get; set; }
        public int Achieved { get; set; }
        public int TotalAchievements { get; set; }
        public double CompletionPercent => TotalAchievements > 0 ? Math.Round((double)Achieved / TotalAchievements * 100, 1) : 0;
        public SteamNewsItem? LatestNews { get; set; }
    }

    public class SteamNewsItem
    {
        public string Title { get; set; } = string.Empty;
        public string Url { get; set; } = string.Empty;
        public string FeedLabel { get; set; } = string.Empty;
        public DateTime Date { get; set; }
    }
}
