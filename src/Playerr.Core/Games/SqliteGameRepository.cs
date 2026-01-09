using Microsoft.EntityFrameworkCore;
using Playerr.Core.Data;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;

namespace Playerr.Core.Games
{
    public class SqliteGameRepository : IGameRepository
    {
        private readonly IDbContextFactory<PlayerrDbContext> _contextFactory;

        public SqliteGameRepository(IDbContextFactory<PlayerrDbContext> contextFactory)
        {
            _contextFactory = contextFactory;
        }

        public async Task<List<Game>> GetAllAsync()
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            return await context.Games
                .Include(g => g.GameFiles)
                .ToListAsync();
        }

        public async Task<Game?> GetByIdAsync(int id)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            return await context.Games
                .Include(g => g.GameFiles)
                .FirstOrDefaultAsync(g => g.Id == id);
        }

        public async Task<Game> AddAsync(Game game)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            context.Games.Add(game);
            await context.SaveChangesAsync();
            return game;
        }

        public async Task<Game?> UpdateAsync(int id, Game game)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var existing = await context.Games.FindAsync(id);
            if (existing == null) return null;

            context.Entry(existing).CurrentValues.SetValues(game);
            
            // Handle lists and owned types separately
            existing.Genres = game.Genres;
            existing.Images = game.Images;
            
            await context.SaveChangesAsync();
            return existing;
        }

        public async Task<bool> DeleteAsync(int id)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var game = await context.Games.FindAsync(id);
            if (game == null) return false;

            context.Games.Remove(game);
            await context.SaveChangesAsync();
            return true;
        }

        public async Task<int> DeleteSteamGamesAsync()
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var steamGames = await context.Games.Where(g => g.SteamId.HasValue && g.SteamId > 0).ToListAsync();
            context.Games.RemoveRange(steamGames);
            await context.SaveChangesAsync();
            return steamGames.Count;
        }

        public async Task DeleteAllAsync()
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            // Using ExecuteDeleteAsync for efficiency if available (EF Core 7+), otherwise standard RemoveRange
            // For safety and compatibility with older EF Core versions in this project stack:
            var allGames = await context.Games.ToListAsync();
            context.Games.RemoveRange(allGames);
            await context.SaveChangesAsync();
        }

        public async Task<int?> GetPlatformIdBySlugAsync(string slug)
        {
            using var context = await _contextFactory.CreateDbContextAsync();
            var platform = await context.Platforms
                .FirstOrDefaultAsync(p => p.Slug == slug);
            return platform?.Id;
        }
    }
}
