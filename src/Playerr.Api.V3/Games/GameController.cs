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
        public async Task<ActionResult<Game>> Update(int id, [FromBody] Game game)
        {
            var updated = await _repository.UpdateAsync(id, game);
            if (updated == null)
            {
                return NotFound();
            }

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

        [HttpDelete("all")]
        public async Task<ActionResult> DeleteAll()
        {
            await _repository.DeleteAllAsync();
            return NoContent();
        }
    }
}
