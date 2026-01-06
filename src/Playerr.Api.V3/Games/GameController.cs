using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Games;
using Playerr.Core.MetadataSource;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Linq;

namespace Playerr.Api.V3.Games
{
    [ApiController]
    [Route("api/v3/[controller]")]
    public class GameController : ControllerBase
    {
        private readonly IGameRepository _repository;
        private readonly IGameMetadataServiceFactory _metadataServiceFactory;

        public GameController(IGameRepository repository, IGameMetadataServiceFactory metadataServiceFactory)
        {
            _repository = repository;
            _metadataServiceFactory = metadataServiceFactory;
        }

        [HttpGet]
        public async Task<ActionResult<List<Game>>> GetAll()
        {
            var games = await _repository.GetAllAsync();
            return Ok(games);
        }

        [HttpGet("{id}")]
        public async Task<ActionResult<Game>> GetById(int id, [FromQuery] string? lang = null)
        {
            var game = await _repository.GetByIdAsync(id);
            if (game == null)
            {
                return NotFound();
            }

            // If a language is requested and the game has an IgdbId, fetch localized metadata
            if (!string.IsNullOrEmpty(lang) && game.IgdbId.HasValue)
            {
                try
                {
                    var metadataService = _metadataServiceFactory.CreateService();
                    var localizedGame = await metadataService.GetGameMetadataAsync(game.IgdbId.Value, lang);
                    
                    if (localizedGame != null)
                    {
                        // Override localized fields for the display
                        game.Title = localizedGame.Title;
                        game.Overview = localizedGame.Overview;
                        game.Storyline = localizedGame.Storyline;
                        game.Genres = localizedGame.Genres;
                        if (game.Platform != null)
                        {
                            game.Platform.Name = metadataService.LocalizePlatform(game.Platform.Name, lang);
                        }
                    }
                }
                catch
                {
                    // Fallback to stored metadata if IGDB fetch fails
                }
            }

            return Ok(game);
        }

        [HttpPost]
        public async Task<ActionResult<Game>> Create([FromBody] Game game)
        {
            var created = await _repository.AddAsync(game);
            return CreatedAtAction(nameof(GetById), new { id = created.Id }, created);
        }

        [HttpPut("{id}")]
        public async Task<ActionResult<Game>> Update(int id, [FromBody] Game gameUpdate)
        {
            var existingGame = await _repository.GetByIdAsync(id);
            if (existingGame == null)
            {
                return NotFound();
            }

            // Check if IGDB ID has changed
            bool igdbIdChanged = gameUpdate.IgdbId.HasValue && gameUpdate.IgdbId != existingGame.IgdbId;

            // Apply updates
            if (gameUpdate.IgdbId.HasValue) existingGame.IgdbId = gameUpdate.IgdbId;
            if (!string.IsNullOrEmpty(gameUpdate.Title)) existingGame.Title = gameUpdate.Title;
            if (!string.IsNullOrEmpty(gameUpdate.InstallPath)) existingGame.InstallPath = gameUpdate.InstallPath;
            // Add other fields as necessary if the frontend sends them

            // If IGDB ID changed, fetch fresh metadata
            if (igdbIdChanged)
            {
                try
                {
                    var metadataService = _metadataServiceFactory.CreateService();
                    // Fetch in English (or default) to store in DB. Localization happens on GetById.
                    var freshMetadata = await metadataService.GetGameMetadataAsync(existingGame.IgdbId.Value, "en");
                    
                    if (freshMetadata != null) {
                       // Update core metadata
                       existingGame.Title = freshMetadata.Title; 
                       existingGame.Overview = freshMetadata.Overview;
                       existingGame.Storyline = freshMetadata.Storyline;
                       existingGame.Year = freshMetadata.Year;
                       existingGame.ReleaseDate = freshMetadata.ReleaseDate;
                       existingGame.Rating = freshMetadata.Rating;
                       existingGame.Genres = freshMetadata.Genres;
                       
                       // IMAGES are critical!
                       if (freshMetadata.Images != null) {
                           existingGame.Images = freshMetadata.Images;
                       }
                    }
                }
                catch (System.Exception ex)
                {
                    // Log error but proceed with saving the ID change at least?
                    System.Console.WriteLine($"Error refreshing metadata: {ex.Message}");
                }
            }

            var updated = await _repository.UpdateAsync(id, existingGame);
            return Ok(updated);
        }

        [HttpDelete("{id}")]
        public async Task<ActionResult> Delete(int id)
        {
            var removed = await _repository.DeleteAsync(id);
            if (!removed)
            {
                return NotFound();
            }

            return NoContent();
        }

        [HttpPost("{id}/install")]
        public async Task<ActionResult> Install(int id)
        {
            var game = await _repository.GetByIdAsync(id);
            if (game == null) return NotFound("Game not found in repository");

            if (string.IsNullOrEmpty(game.Path)) return BadRequest("Game path is not set.");
            
            string targetPath = game.Path;
            System.Console.WriteLine($"[Install] Target Path: {targetPath}");

            // Case 1: Target is a file
            if (System.IO.File.Exists(targetPath))
            {
                if (System.IO.Path.GetExtension(targetPath).ToLower() == ".exe")
                {
                    return LaunchInstaller(targetPath);
                }
                else
                {
                    return BadRequest($"Path is a file but not an .exe: {targetPath}");
                }
            }

            // Case 2: Target is a directory
            if (System.IO.Directory.Exists(targetPath))
            {
                var exeFiles = System.IO.Directory.GetFiles(targetPath, "*.exe", System.IO.SearchOption.AllDirectories);
                if (exeFiles.Length == 0)
                {
                    return BadRequest($"No .exe files found in directory: {targetPath}");
                }
                else if (exeFiles.Length == 1)
                {
                    return LaunchInstaller(exeFiles[0]);
                }
                else
                {
                    var names = string.Join(", ", exeFiles.Select(System.IO.Path.GetFileName).Take(5));
                    return BadRequest($"Multiple .exe files found ({exeFiles.Length}). Candidates: {names}...");
                }
            }

            return BadRequest($"Path does not exist: {targetPath}");
        }

        private ActionResult LaunchInstaller(string path)
        {
            try 
            {
                System.Console.WriteLine($"[Install] Launching: {path}");
                System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
                {
                    FileName = path,
                    UseShellExecute = true
                });
                return Ok(new { message = "Installer launched", path = path });
            }
            catch (System.Exception ex)
            {
                System.Console.WriteLine($"[Install] Launch error: {ex.Message}");
                return StatusCode(500, $"Error launching installer: {ex.Message}");
            }
        }

        [HttpDelete("all")]
        public async Task<ActionResult> DeleteAll()
        {
            await _repository.DeleteAllAsync();
            return NoContent();
        }
    }
}
