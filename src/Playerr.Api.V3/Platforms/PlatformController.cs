using System.Collections.Generic;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Games;

namespace Playerr.Api.V3.Platforms
{
    [ApiController]
    [Route("api/v3/[controller]")]
    public class PlatformController : ControllerBase
    {
        [HttpGet]
        public ActionResult<List<Platform>> GetAll()
        {
            // Return predefined platforms
            var platforms = new List<Platform>
            {
                new Platform { Id = 1, Name = "PC", Slug = "pc", Type = PlatformType.PC },
                new Platform { Id = 2, Name = "macOS", Slug = "macos", Type = PlatformType.MacOS },
                new Platform { Id = 3, Name = "PlayStation 5", Slug = "ps5", Type = PlatformType.PlayStation5 },
                new Platform { Id = 4, Name = "Xbox Series X", Slug = "xbox-series-x", Type = PlatformType.XboxSeriesX },
                new Platform { Id = 5, Name = "Nintendo Switch", Slug = "switch", Type = PlatformType.Switch },
                new Platform { Id = 6, Name = "PlayStation 4", Slug = "ps4", Type = PlatformType.PlayStation4 },
                new Platform { Id = 7, Name = "Xbox One", Slug = "xbox-one", Type = PlatformType.XboxOne }
            };
            
            return Ok(platforms);
        }

        [HttpGet("{id}")]
        public ActionResult<Platform> GetById(int id)
        {
            // TODO: Implement with repository
            return NotFound();
        }
    }
}
