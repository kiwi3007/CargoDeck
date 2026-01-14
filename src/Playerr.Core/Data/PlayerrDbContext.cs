using Microsoft.EntityFrameworkCore;
using Playerr.Core.Games;
using System.Collections.Generic;
using System.Text.Json;
using System.Linq;
using System;

namespace Playerr.Core.Data
{
    public class PlayerrDbContext : DbContext
    {
        public PlayerrDbContext(DbContextOptions<PlayerrDbContext> options) : base(options)
        {
        }

        public DbSet<Game> Games { get; set; }
        public DbSet<Platform> Platforms { get; set; }
        public DbSet<GameFile> GameFiles { get; set; }

        protected override void OnModelCreating(ModelBuilder modelBuilder)
        {
            base.OnModelCreating(modelBuilder);

            modelBuilder.Entity<Game>(entity =>
            {
                entity.HasKey(e => e.Id);
                entity.Property(e => e.Title).IsRequired();
                
                // Owned type for Images
                entity.OwnsOne(e => e.Images, images =>
                {
                    var stringListComparer = new Microsoft.EntityFrameworkCore.ChangeTracking.ValueComparer<List<string>>(
                        (c1, c2) => c1.SequenceEqual(c2),
                        c => c.Aggregate(0, (a, v) => HashCode.Combine(a, v.GetHashCode())),
                        c => c.ToList());

                    images.Property(i => i.Screenshots)
                        .HasConversion(
                            v => JsonSerializer.Serialize(v, (JsonSerializerOptions)null),
                            v => JsonSerializer.Deserialize<List<string>>(v, (JsonSerializerOptions)null) ?? new List<string>())
                        .Metadata.SetValueComparer(stringListComparer);

                    images.Property(i => i.Artworks)
                        .HasConversion(
                            v => JsonSerializer.Serialize(v, (JsonSerializerOptions)null),
                            v => JsonSerializer.Deserialize<List<string>>(v, (JsonSerializerOptions)null) ?? new List<string>())
                        .Metadata.SetValueComparer(stringListComparer);
                });

                // Genres as JSON string with comparer
                var stringListComparer = new Microsoft.EntityFrameworkCore.ChangeTracking.ValueComparer<List<string>>(
                    (c1, c2) => c1.SequenceEqual(c2),
                    c => c.Aggregate(0, (a, v) => HashCode.Combine(a, v.GetHashCode())),
                    c => c.ToList());

                entity.Property(e => e.Genres)
                    .HasConversion(
                        v => JsonSerializer.Serialize(v, (JsonSerializerOptions)null),
                        v => JsonSerializer.Deserialize<List<string>>(v, (JsonSerializerOptions)null) ?? new List<string>())
                    .Metadata.SetValueComparer(stringListComparer);

                entity.HasMany(e => e.GameFiles)
                    .WithOne(f => f.Game)
                    .HasForeignKey(f => f.GameId);
            });

            modelBuilder.Entity<Platform>(entity =>
            {
                entity.HasKey(e => e.Id);
            });

            modelBuilder.Entity<GameFile>(entity =>
            {
                entity.HasKey(e => e.Id);
            });
        }
    }
}
