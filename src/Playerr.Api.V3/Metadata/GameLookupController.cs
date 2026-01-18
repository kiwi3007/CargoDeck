using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Games;
using Playerr.Core.MetadataSource;

namespace Playerr.Api.V3.Metadata
{
    /// <summary>
    /// Controlador para búsqueda de metadata - Similar a MovieLookupController en Radarr
    /// Permite buscar juegos en IGDB y obtener toda su información visual
    /// </summary>
    [ApiController]
    [Route("api/v3/game/lookup")]
    public class GameLookupController : ControllerBase
    {
        private readonly IGameMetadataServiceFactory _metadataServiceFactory;
        private readonly IGameRepository _gameRepository;

        public GameLookupController(IGameMetadataServiceFactory metadataServiceFactory, IGameRepository gameRepository)
        {
            _metadataServiceFactory = metadataServiceFactory;
            _gameRepository = gameRepository;
        }

        /// <summary>
        /// Buscar juegos por título - Similar a cómo Radarr busca películas
        /// </summary>
        [HttpGet]
        public async Task<ActionResult<List<Game>>> Search([FromQuery] string term, [FromQuery] string? platformKey = null, [FromQuery] string? lang = null)
        {
            if (string.IsNullOrWhiteSpace(term))
            {
                return BadRequest("Search term is required");
            }

            try
            {
                var metadataService = _metadataServiceFactory.CreateService();
                var games = await metadataService.SearchGamesAsync(term, platformKey, lang);

                // Check which of these are already in the library
                var ownedIds = await _gameRepository.GetIgdbIdsAsync();
                foreach (var game in games)
                {
                    if (game.IgdbId.HasValue && ownedIds.Contains(game.IgdbId.Value))
                    {
                        game.IsOwned = true;
                    }
                }

                return Ok(games);
            }
            catch (Exception ex)
            {
                return StatusCode(500, new { error = ex.Message });
            }
        }

        /// <summary>
        /// Obtener información completa de un juego por su IGDB ID
        /// </summary>
        [HttpGet("igdb/{igdbId}")]
        public async Task<ActionResult<Game>> GetByIgdbId(int igdbId, [FromQuery] string? lang = null)
        {
            try
            {
                var metadataService = _metadataServiceFactory.CreateService();
                var game = await metadataService.GetGameMetadataAsync(igdbId, lang);
                
                if (game == null)
                {
                    return NotFound();
                }

                return Ok(game);
            }
            catch (Exception ex)
            {
                return StatusCode(500, new { error = ex.Message });
            }
        }
    }
}
