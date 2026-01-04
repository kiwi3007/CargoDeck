using Playerr.Core.MetadataSource.Steam;
using Playerr.Core.MetadataSource.Igdb;
using Playerr.Core.Games;
using System.Text.RegularExpressions;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using System.IO;
using System;

namespace Playerr.Core.MetadataSource
{
    /// <summary>
    /// Servicio para obtener metadata de juegos - Similar a MovieInfoService en Radarr
    /// </summary>
    public class GameMetadataService
    {
        private readonly IgdbClient _igdbClient;
        private readonly SteamClient _steamClient;

        public GameMetadataService(IgdbClient igdbClient, SteamClient steamClient)
        {
            _igdbClient = igdbClient;
            _steamClient = steamClient;
        }

        public async Task<List<Game>> SearchGamesAsync(string query, string? platformKey = null, string? lang = null, string? serial = null)
        {
            int? igdbPlatformId = null;
            if (!string.IsNullOrEmpty(platformKey))
            {
                igdbPlatformId = platformKey.ToLower() switch
                {
                    "nintendo_switch" => 130,
                    "ps4" => 48,
                    "ps3" => 9,
                    "ps5" => 167,
                    "pc_windows" => 6,
                    "macos" => 14,
                    "retro_emulation" => null, // Too many to list, let IGDB search
                    _ => null
                };
            }

            var igdbGames = await _igdbClient.SearchGamesAsync(query, igdbPlatformId, lang, serial);
            return igdbGames.Select(g => MapIgdbGameToGame(g, lang)).ToList();
        }

        public async Task<Game?> GetGameMetadataAsync(int igdbId, string? lang = null)
        {
            var results = await GetGamesMetadataAsync(new[] { igdbId }, lang);
            return results.FirstOrDefault();
        }

        public async Task<List<Game>> GetGamesMetadataAsync(IEnumerable<int> igdbIds, string? lang = null)
        {
            var igdbGames = await _igdbClient.GetGamesByIdsAsync(igdbIds, lang);
            // Process sequentially to avoid internal rate limit issues if Steam is called
            var results = new List<Game>();
            foreach (var igdbGame in igdbGames)
            {
                results.Add(MapIgdbGameToGame(igdbGame, lang));
            }
            return results;
        }

        private Game MapIgdbGameToGame(IgdbGame igdbGame, string? lang = null)
        {
            var title = igdbGame.Name;
            var summary = igdbGame.Summary;
            var storyline = igdbGame.Storyline;

            if (!string.IsNullOrEmpty(lang))
            {
                var targetLangId = GetIgdbLanguageId(lang);
                if (targetLangId.HasValue)
                {
                    // IGDB mostly only provides localized titles
                    var localization = igdbGame.Localizations.FirstOrDefault(l => l.Language == targetLangId.Value);
                    if (localization != null && !string.IsNullOrEmpty(localization.Name))
                    {
                        title = localization.Name;
                    }
                }

                // Try to get summary from Steam if available
                var steamId = igdbGame.ExternalGames.FirstOrDefault(eg => eg.Category == 1)?.Uid;
                if (!string.IsNullOrEmpty(steamId))
                {
                    // Category 1 is Steam in IGDB
                    var steamDetails = _steamClient.GetGameDetailsAsync(steamId, lang).GetAwaiter().GetResult();
                    if (steamDetails != null)
                    {
                        if (!string.IsNullOrEmpty(steamDetails.AboutTheGame))
                        {
                            summary = CleanHtml(steamDetails.AboutTheGame);
                        }
                        else if (!string.IsNullOrEmpty(steamDetails.DetailedDescription))
                        {
                            summary = CleanHtml(steamDetails.DetailedDescription);
                        }
                    }
                }
            }

            var game = new Game
            {
                Title = title,
                Overview = summary,
                Storyline = storyline,
                IgdbId = igdbGame.Id,
                Rating = igdbGame.Rating,
                RatingCount = igdbGame.RatingCount,
                Genres = igdbGame.Genres.Select(g => LocalizeGenre(g.Name, lang)).ToList(),
                Images = new GameImages()
            };

            // Release date
            if (igdbGame.FirstReleaseDate.HasValue)
            {
                game.ReleaseDate = DateTimeOffset.FromUnixTimeSeconds(igdbGame.FirstReleaseDate.Value).DateTime;
                game.Year = game.ReleaseDate.Value.Year;
            }

            // Developer & Publisher
            var developer = igdbGame.InvolvedCompanies.FirstOrDefault(c => c.Developer);
            var publisher = igdbGame.InvolvedCompanies.FirstOrDefault(c => c.Publisher);
            game.Developer = developer?.Company.Name;
            game.Publisher = publisher?.Company.Name;

            // Cover/Poster - Similar a Radarr que tiene poster principal
            if (igdbGame.Cover != null)
            {
                game.Images.CoverUrl = IgdbClient.GetImageUrl(igdbGame.Cover.ImageId, ImageSize.CoverBig);
                game.Images.CoverLargeUrl = IgdbClient.GetImageUrl(igdbGame.Cover.ImageId, ImageSize.HD);
            }

            // Screenshots - Como Radarr muestra screenshots de películas
            game.Images.Screenshots = igdbGame.Screenshots
                .Select(s => IgdbClient.GetImageUrl(s.ImageId, ImageSize.ScreenshotHuge))
                .ToList();

            // Artworks - Arte conceptual, similar a fanart en Radarr
            game.Images.Artworks = igdbGame.Artworks
                .Select(a => IgdbClient.GetImageUrl(a.ImageId, ImageSize.HD))
                .ToList();

            // Background - Usar el primer artwork o screenshot como fondo
            game.Images.BackgroundUrl = game.Images.Artworks.FirstOrDefault() 
                                       ?? game.Images.Screenshots.FirstOrDefault();

            return game;
        }

        private int? GetIgdbLanguageId(string lang)
        {
            return lang.ToLower() switch
            {
                "en" => 1,
                "zh" => 2,
                "fr" => 3,
                "de" => 4,
                "ja" => 6,
                "ru" => 9,
                "es" => 10,
                _ => null
            };
        }

        private string CleanHtml(string html)
        {
            if (string.IsNullOrEmpty(html)) return html;
            // Basic HTML cleaning
            var step1 = Regex.Replace(html, "<.*?>", string.Empty);
            var step2 = System.Net.WebUtility.HtmlDecode(step1);
            return step2.Trim();
        }

        private string LocalizeGenre(string genre, string? lang)
        {
            if (string.IsNullOrEmpty(lang) || lang == "en") return genre;

            var mappings = new Dictionary<string, Dictionary<string, string>>
            {
                ["es"] = new() {
                    ["Adventure"] = "Aventura", ["Role-playing (RPG)"] = "RPG", ["Shooter"] = "Disparos",
                    ["Fighting"] = "Lucha", ["Indie"] = "Indie", ["Racing"] = "Carreras",
                    ["Sport"] = "Deportes", ["Simulator"] = "Simulador", ["Strategy"] = "Estrategia",
                    ["Arcade"] = "Arcade", ["Platform"] = "Plataformas", ["Puzzle"] = "Puzle",
                    ["Music"] = "Música", ["Tactical"] = "Táctico", ["Turn-based strategy (TBS)"] = "Estrategia por turnos",
                    ["Real Time Strategy (RTS)"] = "Estrategia en tiempo real", ["Hack and slash/Beat 'em up"] = "Hack and slash",
                    ["Pinball"] = "Pinball", ["Point-and-click"] = "Point-and-click", ["Quiz/Trivia"] = "Preguntas",
                    ["Visual Novel"] = "Novela Visual", ["Card & Board Game"] = "Cartas y Tablero", ["MOBA"] = "MOBA"
                },
                ["fr"] = new() {
                    ["Adventure"] = "Aventure", ["Role-playing (RPG)"] = "RPG", ["Shooter"] = "Tir",
                    ["Fighting"] = "Combat", ["Indie"] = "Indé", ["Racing"] = "Course",
                    ["Sport"] = "Sport", ["Simulator"] = "Simulateur", ["Strategy"] = "Stratégie",
                    ["Arcade"] = "Arcade", ["Platform"] = "Plateforme", ["Puzzle"] = "Puzzle",
                    ["Music"] = "Musique", ["Tactical"] = "Tactique", ["Real Time Strategy (RTS)"] = "Stratégie en temps réel",
                    ["Hack and slash/Beat 'em up"] = "Hack and slash"
                },
                ["de"] = new() {
                    ["Adventure"] = "Abenteuer", ["Role-playing (RPG)"] = "Rollenspiel", ["Shooter"] = "Shooter",
                    ["Fighting"] = "Kampfspiel", ["Indie"] = "Indie", ["Racing"] = "Rennspiel",
                    ["Sport"] = "Sport", ["Simulator"] = "Simulator", ["Strategy"] = "Strategie",
                    ["Puzzle"] = "Rätsel", ["Platform"] = "Plattform"
                },
                ["ru"] = new() {
                    ["Adventure"] = "Приключения", ["Role-playing (RPG)"] = "RPG", ["Shooter"] = "Шутер",
                    ["Fighting"] = "Файтинг", ["Indie"] = "Инди", ["Racing"] = "Гонки",
                    ["Sport"] = "Спорт", ["Simulator"] = "Симулятор", ["Strategy"] = "Стратегия",
                    ["Puzzle"] = "Головоломка", ["Platform"] = "Платформер"
                },
                ["zh"] = new() {
                    ["Adventure"] = "冒险", ["Role-playing (RPG)"] = "角色扮演", ["Shooter"] = "射击",
                    ["Fighting"] = "格斗", ["Indie"] = "独立", ["Racing"] = "竞速",
                    ["Sport"] = "体育", ["Simulator"] = "模拟", ["Strategy"] = "策略",
                    ["Puzzle"] = "解谜", ["Platform"] = "平台"
                },
                ["ja"] = new() {
                    ["Adventure"] = "アドベンチャー", ["Role-playing (RPG)"] = "ロールプレイング", ["Shooter"] = "シューティング",
                    ["Fighting"] = "格闘", ["Indie"] = "インディー", ["Racing"] = "レース",
                    ["Sport"] = "スポーツ", ["Simulator"] = "シミュレーター", ["Strategy"] = "ストラテジー",
                    ["Puzzle"] = "パズル", ["Platform"] = "プラットフォーム"
                }
            };

            if (mappings.TryGetValue(lang.ToLower(), out var langMappings) && langMappings.TryGetValue(genre, out var localized))
            {
                return localized;
            }

            return genre;
        }

        public string LocalizePlatform(string platform, string? lang)
        {
            if (string.IsNullOrEmpty(lang) || lang == "en") return platform;

            var mappings = new Dictionary<string, Dictionary<string, string>>
            {
                ["es"] = new() { ["PC (Microsoft Windows)"] = "PC", ["PlayStation 5"] = "PS5", ["PlayStation 4"] = "PS4", ["Xbox Series X|S"] = "Xbox Series" },
                ["fr"] = new() { ["PC (Microsoft Windows)"] = "PC" },
                ["de"] = new() { ["PC (Microsoft Windows)"] = "PC" },
                ["ru"] = new() { ["PC (Microsoft Windows)"] = "PC" },
                ["zh"] = new() { ["PC (Microsoft Windows)"] = "PC" },
                ["ja"] = new() { ["PC (Microsoft Windows)"] = "PC" }
            };

            if (mappings.TryGetValue(lang.ToLower(), out var langMappings) && langMappings.TryGetValue(platform, out var localized))
            {
                return localized;
            }

            return platform;
        }
    }
}
