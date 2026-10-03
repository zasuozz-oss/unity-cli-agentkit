// FxBurst.cs — copy into the game's runtime scripts and set the namespace.
using UnityEngine;

namespace Game
{
    /// <summary>One-shot particle prefab: spawned, played once, destroyed when its last particle is gone.</summary>
    public static class FxBurst
    {
        // Pack effects built for a looping popup (a sunburst is often one 100 s particle) are squeezed to a one-shot.
        private const float MaxLife = 3f;

        /// <summary>Instantiates <paramref name="prefab"/> under <paramref name="parent"/> at world <paramref name="position"/>;
        /// null prefab, or not in Play mode (timed Destroy is unavailable there and the scene would be dirtied) → null.</summary>
        public static GameObject Play(GameObject prefab, Transform parent, Vector3 position)
        {
            if (prefab == null || !Application.isPlaying) return null;
            var go = Object.Instantiate(prefab, position, prefab.transform.rotation, parent);
            float life = 0f;
            foreach (var ps in go.GetComponentsInChildren<ParticleSystem>(true))
            {
                ps.Stop(true, ParticleSystemStopBehavior.StopEmittingAndClear); // playOnAwake already started it
                var main = ps.main;
                main.loop = false;
                if (main.duration > MaxLife) main.duration = MaxLife;
                if (main.startLifetime.mode == ParticleSystemCurveMode.Constant && main.startLifetime.constant > MaxLife) main.startLifetime = MaxLife;
                life = Mathf.Max(life, main.startDelay.constantMax + main.duration + Mathf.Min(main.startLifetime.constantMax, MaxLife));
            }
            go.GetComponent<ParticleSystem>()?.Play(true);
            Object.Destroy(go, life + 0.5f); // timed destroy, not stopAction: covers child systems and trails
            return go;
        }

        /// <summary>World-space centre of a UI element (its pivot may sit on an edge).</summary>
        public static Vector3 Center(Component c) =>
            c.transform is RectTransform rt ? rt.TransformPoint(rt.rect.center) : c.transform.position;

        /// <summary>World point → the same screen point on <paramref name="canvas"/> (Screen Space - Camera), as a world position for <see cref="Play"/>.</summary>
        public static Vector3 ToCanvas(Canvas canvas, Camera worldCam, Vector3 worldPos)
        {
            var screen = RectTransformUtility.WorldToScreenPoint(worldCam, worldPos);
            RectTransformUtility.ScreenPointToWorldPointInRectangle((RectTransform)canvas.transform, screen, canvas.worldCamera, out var p);
            return p;
        }
    }
}
